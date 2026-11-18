use anyhow::Result;
use clap::ValueEnum;
use serde_json::Value;

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, ValueEnum)]
pub enum OutputFormat {
    #[default]
    Text,
    Json,
}

pub fn print_value(fmt: OutputFormat, value: &Value) -> Result<()> {
    match fmt {
        OutputFormat::Json => {
            println!("{}", serde_json::to_string_pretty(value)?);
        }
        OutputFormat::Text => {
            println!("{}", text_preview(value));
        }
    }
    Ok(())
}

fn text_preview(value: &Value) -> String {
    match value {
        Value::Object(map) => {
            if let Some(Value::Array(items)) = map.values().find(|v| v.is_array()) {
                return items.iter().map(row_line).collect::<Vec<_>>().join("\n");
            }
            map.iter()
                .map(|(k, v)| format!("{k}: {}", compact(v)))
                .collect::<Vec<_>>()
                .join("\n")
        }
        Value::Array(items) => items.iter().map(row_line).collect::<Vec<_>>().join("\n"),
        other => compact(other),
    }
}

fn row_line(value: &Value) -> String {
    match value {
        Value::Object(map) => {
            let id = map
                .get("id")
                .or_else(|| map.get("name"))
                .map(compact)
                .unwrap_or_else(|| compact(value));
            let name = map.get("name").map(compact).unwrap_or_default();
            let status = map
                .get("status")
                .or_else(|| map.get("role"))
                .map(compact)
                .unwrap_or_default();
            let extra = map
                .get("endpoint")
                .or_else(|| map.get("hostname"))
                .or_else(|| map.get("imageOrSource"))
                .or_else(|| map.get("image_or_source"))
                .map(compact)
                .unwrap_or_default();
            [id, name, status, extra]
                .into_iter()
                .filter(|s| !s.is_empty())
                .collect::<Vec<_>>()
                .join("\t")
        }
        other => compact(other),
    }
}

fn compact(value: &Value) -> String {
    match value {
        Value::String(s) => s.clone(),
        Value::Number(n) => n.to_string(),
        Value::Bool(b) => b.to_string(),
        Value::Null => String::new(),
        other => other.to_string(),
    }
}
