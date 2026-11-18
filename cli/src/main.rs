mod client;
mod config;
mod output;

use std::io::{self, Write};
use std::thread;
use std::time::Duration;

use anyhow::{bail, Context, Result};
use clap::{CommandFactory, Parser, Subcommand};
use clap_complete::{generate, Shell};
use serde_json::{json, Value};

use client::ApiClient;
use output::{print_value, OutputFormat};

#[derive(Debug, Parser)]
#[command(
    name = "norix",
    version,
    about = "Norix control-plane CLI (Rust) — deploy, logs, nodes, and more"
)]
struct Cli {
    /// Control plane base URL (no /api suffix)
    #[arg(
        long,
        global = true,
        env = "NORIX_API",
        default_value = "http://127.0.0.1:8080"
    )]
    api: String,

    /// Session bearer token (falls back to ~/.norix/token)
    #[arg(long, global = true, env = "NORIX_TOKEN")]
    token: Option<String>,

    #[arg(long, global = true, value_enum, default_value_t = OutputFormat::Text)]
    output: OutputFormat,

    #[command(subcommand)]
    command: Commands,
}

#[derive(Debug, Subcommand)]
enum Commands {
    /// GET /healthz
    Health,
    /// Store API token in ~/.norix/token
    Login {
        #[arg(long)]
        token: Option<String>,
    },
    /// Project commands
    Projects {
        #[command(subcommand)]
        command: ProjectsCmd,
    },
    /// List or inspect workloads
    Workloads {
        #[command(subcommand)]
        command: WorkloadsCmd,
    },
    /// Ask the agent to remove the container; the workload row stays
    Stop {
        #[arg(long)]
        workload_id: String,
    },
    /// Create (optional) and deploy a workload image
    Deploy {
        #[arg(long)]
        project: String,
        #[arg(long)]
        name: String,
        #[arg(long)]
        image: Option<String>,
        #[arg(long)]
        workload_id: Option<String>,
        #[arg(long, default_value_t = 80)]
        port: i32,
    },
    /// Stream or tail workload runtime logs
    Logs {
        #[arg(long)]
        workload_id: String,
        #[arg(long, default_value_t = 100)]
        tail: i32,
        /// Follow new lines (poll)
        #[arg(short = 'f', long)]
        follow: bool,
        #[arg(long, default_value_t = 2)]
        interval: u64,
    },
    /// Reveal plaintext env (owner/admin; audited)
    Env {
        #[arg(long)]
        workload_id: String,
    },
    /// List domains for a project
    Domains {
        #[arg(long)]
        project: String,
    },
    /// List enrolled nodes
    Nodes,
    /// Roll back a workload to a release
    Rollback {
        #[arg(long)]
        workload_id: String,
        #[arg(long)]
        release_id: Option<String>,
        /// Use the previous release (index 1) when --release-id is omitted
        #[arg(long, default_value_t = 1)]
        previous: usize,
    },
    /// List release history for a workload
    Releases {
        #[arg(long)]
        workload_id: String,
        #[arg(long, default_value_t = 20)]
        limit: i32,
    },
    /// Print shell completions
    Completions {
        #[arg(value_enum)]
        shell: Shell,
    },
}

#[derive(Debug, Subcommand)]
enum ProjectsCmd {
    List,
}

#[derive(Debug, Subcommand)]
enum WorkloadsCmd {
    List {
        #[arg(long)]
        project: String,
    },
}

fn main() {
    if let Err(error) = run() {
        eprintln!("error: {error:#}");
        std::process::exit(1);
    }
}

fn run() -> Result<()> {
    let cli = Cli::parse();
    if let Commands::Completions { shell } = cli.command {
        let mut cmd = Cli::command();
        generate(shell, &mut cmd, "norix", &mut io::stdout());
        return Ok(());
    }

    let token = cli
        .token
        .clone()
        .filter(|t| !t.trim().is_empty())
        .or_else(config::read_token);
    let client = ApiClient::new(&cli.api, token)?;

    match cli.command {
        Commands::Health => {
            let (status, body) = client.healthz()?;
            match cli.output {
                OutputFormat::Json => {
                    print_value(cli.output, &json!({"status": status, "body": body}))?
                }
                OutputFormat::Text => println!("{status} {body}"),
            }
            if status >= 300 {
                bail!("unhealthy");
            }
        }
        Commands::Login { token } => {
            let token = token
                .or(cli.token.clone())
                .or_else(|| std::env::var("NORIX_TOKEN").ok())
                .context("pass --token or set NORIX_TOKEN")?;
            let path = config::write_token(&token)?;
            println!("Saved token to {}", path.display());
        }
        Commands::Projects {
            command: ProjectsCmd::List,
        } => {
            client.require_token()?;
            let body = client.connect_json("/v1.ProjectService/ListProjects", &json!({}))?;
            print_value(cli.output, &body)?;
        }
        Commands::Workloads {
            command: WorkloadsCmd::List { project },
        } => {
            client.require_token()?;
            let body = client.connect_json(
                "/v1.WorkloadService/ListWorkloads",
                &json!({ "projectId": project }),
            )?;
            print_value(cli.output, &body)?;
        }
        Commands::Stop { workload_id } => {
            client.require_token()?;
            let body = client.connect_json(
                "/v1.WorkloadService/StopWorkload",
                &json!({ "workloadId": workload_id }),
            )?;
            print_value(cli.output, &body)?;
        }
        Commands::Deploy {
            project,
            name,
            image,
            workload_id,
            port,
        } => {
            client.require_token()?;
            let id = if let Some(id) = workload_id {
                id
            } else {
                let image = image.context("--image is required when creating a new workload")?;
                let created = client.connect_json(
                    "/v1.WorkloadService/CreateWorkload",
                    &json!({
                        "workload": {
                            "projectId": project,
                            "name": name,
                            "imageOrSource": image,
                            "containerPort": port,
                            "sourceKind": "image",
                            "replicaCount": 1,
                            "memoryLimitMb": "512",
                            "cpuShares": 1024
                        }
                    }),
                )?;
                created
                    .pointer("/workload/id")
                    .and_then(Value::as_str)
                    .context("create workload: missing id")?
                    .to_string()
            };
            let deployed = client.connect_json(
                "/v1.WorkloadService/Deploy",
                &json!({
                    "workload": { "id": id },
                    "containerPort": port
                }),
            )?;
            print_value(cli.output, &deployed)?;
        }
        Commands::Logs {
            workload_id,
            tail,
            follow,
            interval,
        } => {
            client.require_token()?;
            let mut seen = 0usize;
            loop {
                let body = client.connect_json(
                    "/v1.WorkloadService/GetWorkloadLogs",
                    &json!({ "workloadId": workload_id, "tailLines": tail }),
                )?;
                let lines = body
                    .get("lines")
                    .and_then(Value::as_array)
                    .cloned()
                    .unwrap_or_default();
                if cli.output == OutputFormat::Json && !follow {
                    print_value(cli.output, &body)?;
                    break;
                }
                if lines.len() >= seen {
                    for line in lines.iter().skip(seen) {
                        if let Some(text) = line.as_str() {
                            println!("{text}");
                        }
                    }
                    seen = lines.len();
                } else {
                    // Log buffer reset / shorter tail — print all.
                    for line in &lines {
                        if let Some(text) = line.as_str() {
                            println!("{text}");
                        }
                    }
                    seen = lines.len();
                }
                let _ = io::stdout().flush();
                if !follow {
                    break;
                }
                thread::sleep(Duration::from_secs(interval.max(1)));
            }
        }
        Commands::Env { workload_id } => {
            client.require_token()?;
            let body = client.connect_json(
                "/v1.WorkloadService/RevealWorkloadEnv",
                &json!({ "workloadId": workload_id }),
            )?;
            print_value(cli.output, &body)?;
        }
        Commands::Domains { project } => {
            client.require_token()?;
            let body = client.connect_json(
                "/v1.WorkloadService/ListDomains",
                &json!({ "projectId": project }),
            )?;
            print_value(cli.output, &body)?;
        }
        Commands::Nodes => {
            client.require_token()?;
            let body = client.connect_json("/v1.AgentService/ListNodes", &json!({}))?;
            print_value(cli.output, &body)?;
        }
        Commands::Releases { workload_id, limit } => {
            client.require_token()?;
            let body = client.connect_json(
                "/v1.WorkloadService/ListWorkloadReleases",
                &json!({ "workloadId": workload_id, "limit": limit }),
            )?;
            print_value(cli.output, &body)?;
        }
        Commands::Rollback {
            workload_id,
            release_id,
            previous,
        } => {
            client.require_token()?;
            let release_id = if let Some(id) = release_id {
                id
            } else {
                let body = client.connect_json(
                    "/v1.WorkloadService/ListWorkloadReleases",
                    &json!({ "workloadId": workload_id, "limit": 20 }),
                )?;
                let releases = body
                    .get("releases")
                    .and_then(Value::as_array)
                    .context("no releases")?;
                releases
                    .get(previous)
                    .and_then(|r| r.get("id"))
                    .and_then(Value::as_str)
                    .with_context(|| format!("release index {previous} not found (0 = latest)"))?
                    .to_string()
            };
            let body = client.connect_json(
                "/v1.WorkloadService/RollbackWorkload",
                &json!({ "workloadId": workload_id, "releaseId": release_id }),
            )?;
            print_value(cli.output, &body)?;
        }
        Commands::Completions { .. } => unreachable!(),
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use httpmock::prelude::*;

    #[test]
    fn health_ok() {
        let server = MockServer::start();
        let mock = server.mock(|when, then| {
            when.method(GET).path("/healthz");
            then.status(200).body("ok");
        });
        let client = ApiClient::new(&server.base_url(), None).unwrap();
        let (status, body) = client.healthz().unwrap();
        assert_eq!(status, 200);
        assert_eq!(body, "ok");
        mock.assert();
    }

    #[test]
    fn list_projects_connect_path() {
        let server = MockServer::start();
        let mock = server.mock(|when, then| {
            when.method(POST)
                .path("/api/v1.ProjectService/ListProjects")
                .header("Connect-Protocol-Version", "1")
                .header("Authorization", "Bearer test-token");
            then.status(200)
                .header("content-type", "application/json")
                .body(r#"{"projects":[{"id":"p1","name":"demo"}]}"#);
        });
        let client = ApiClient::new(&server.base_url(), Some("test-token".into())).unwrap();
        let body = client
            .connect_json("/v1.ProjectService/ListProjects", &json!({}))
            .unwrap();
        assert_eq!(body["projects"][0]["name"], "demo");
        mock.assert();
    }

    #[test]
    fn deploy_posts_workload_id() {
        let server = MockServer::start();
        let mock = server.mock(|when, then| {
            when.method(POST)
                .path("/api/v1.WorkloadService/Deploy")
                .json_body(json!({"workload":{"id":"w1"},"containerPort":80}));
            then.status(200)
                .body(r#"{"deploymentId":"d1","status":"deploying","hostPort":18080}"#);
        });
        let client = ApiClient::new(&server.base_url(), Some("t".into())).unwrap();
        let body = client
            .connect_json(
                "/v1.WorkloadService/Deploy",
                &json!({"workload":{"id":"w1"},"containerPort":80}),
            )
            .unwrap();
        assert_eq!(body["hostPort"], 18080);
        mock.assert();
    }

    #[test]
    fn normalize_strips_slash() {
        assert_eq!(
            config::normalize_api_base("http://127.0.0.1:8080/"),
            "http://127.0.0.1:8080"
        );
    }

    #[test]
    fn output_text_lists_rows() {
        let value = json!({"projects":[{"id":"1","name":"a","status":"ok"}]});
        print_value(OutputFormat::Text, &value).unwrap();
    }

    #[test]
    fn output_json_pretty() {
        let value = json!({"ok": true});
        print_value(OutputFormat::Json, &value).unwrap();
    }
}
