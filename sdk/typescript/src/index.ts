export type NorixClientOptions = {
  baseUrl: string;
  token?: string;
};

export class NorixClient {
  private readonly baseUrl: string;
  private readonly token?: string;

  constructor(options: NorixClientOptions) {
    this.baseUrl = options.baseUrl.replace(/\/$/, "");
    this.token = options.token;
  }

  async listProjects(): Promise<unknown> {
    return this.connect("v1.ProjectService/ListProjects", {});
  }

  async listWorkloads(projectId: string): Promise<unknown> {
    return this.connect("v1.WorkloadService/ListWorkloads", { projectId });
  }

  private async connect<T>(procedure: string, body: Record<string, unknown>): Promise<T> {
  const url = `${this.baseUrl}/api/${procedure}`;
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      "Connect-Protocol-Version": "1",
    };
    if (this.token) {
      headers.Authorization = `Bearer ${this.token}`;
    }
    const response = await fetch(url, {
      method: "POST",
      headers,
      body: JSON.stringify(body),
    });
    if (!response.ok) {
      throw new Error(`Norix API ${procedure} failed: ${response.status}`);
    }
    return (await response.json()) as T;
  }
}
