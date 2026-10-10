import * as vscode from "vscode";
import { ChildProcess, spawn, execFile } from "child_process";

export interface ComponentInfo {
  name: string;
  type: string; // "terraform", "helmfile", etc.
  stacks: string[];
}

export interface AtmosClient {
  listStacks(): Promise<string[]>;
  describeComponent(component: string, stack: string): Promise<string>;
  listComponents(): Promise<ComponentInfo[]>;
  dispose(): void;
}

// MCP JSON-RPC message types.
interface JsonRpcRequest {
  jsonrpc: "2.0";
  id: number;
  method: string;
  params?: Record<string, unknown>;
}

interface JsonRpcResponse {
  jsonrpc: "2.0";
  id: number;
  result?: unknown;
  error?: { code: number; message: string; data?: unknown };
}

/**
 * MCP-based atmos client. Spawns `atmos mcp start` as a long-running subprocess
 * and communicates via JSON-RPC over stdio.
 */
class McpAtmosClient implements AtmosClient {
  private process: ChildProcess | undefined;
  private requestId = 0;
  private pendingRequests = new Map<
    number,
    {
      resolve: (value: unknown) => void;
      reject: (reason: Error) => void;
    }
  >();
  private buffer = "";
  private initialized = false;
  private initPromise: Promise<void> | undefined;
  private outputChannel: vscode.OutputChannel;

  constructor(
    private atmosBinary: string,
    private workspaceRoot: string,
    outputChannel: vscode.OutputChannel
  ) {
    this.outputChannel = outputChannel;
  }

  async connect(): Promise<void> {
    if (this.initPromise) {
      return this.initPromise;
    }

    this.initPromise = this.doConnect();
    return this.initPromise;
  }

  private async doConnect(): Promise<void> {
    this.process = spawn(this.atmosBinary, ["mcp", "start"], {
      cwd: this.workspaceRoot,
      stdio: ["pipe", "pipe", "pipe"],
    });

    this.process.stdout?.on("data", (data: Buffer) => {
      this.handleData(data.toString());
    });

    this.process.stderr?.on("data", (data: Buffer) => {
      this.outputChannel.appendLine(`[MCP stderr] ${data.toString().trim()}`);
    });

    this.process.on("error", (err) => {
      this.outputChannel.appendLine(`[MCP] Process error: ${err.message}`);
      this.rejectAllPending(err);
    });

    this.process.on("exit", (code) => {
      this.outputChannel.appendLine(`[MCP] Process exited with code ${code}`);
      this.initialized = false;
      this.initPromise = undefined;
      this.rejectAllPending(new Error(`MCP process exited with code ${code}`));
    });

    // Send MCP initialize request.
    await this.sendRequest("initialize", {
      protocolVersion: "2025-03-26",
      capabilities: {},
      clientInfo: { name: "atmos-vscode", version: "0.1.0" },
    });

    // Send initialized notification.
    this.sendNotification("notifications/initialized", {});
    this.initialized = true;
  }

  private handleData(data: string): void {
    this.buffer += data;

    // MCP uses newline-delimited JSON.
    const lines = this.buffer.split("\n");
    this.buffer = lines.pop() || "";

    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed) continue;

      try {
        const msg = JSON.parse(trimmed) as JsonRpcResponse;
        if (msg.id !== undefined && this.pendingRequests.has(msg.id)) {
          const pending = this.pendingRequests.get(msg.id)!;
          this.pendingRequests.delete(msg.id);

          if (msg.error) {
            pending.reject(
              new Error(`MCP error ${msg.error.code}: ${msg.error.message}`)
            );
          } else {
            pending.resolve(msg.result);
          }
        }
      } catch {
        this.outputChannel.appendLine(`[MCP] Failed to parse: ${trimmed}`);
      }
    }
  }

  private sendRequest(
    method: string,
    params?: Record<string, unknown>
  ): Promise<unknown> {
    return new Promise((resolve, reject) => {
      if (!this.process?.stdin?.writable) {
        reject(new Error("MCP process not connected"));
        return;
      }

      const id = ++this.requestId;
      const request: JsonRpcRequest = {
        jsonrpc: "2.0",
        id,
        method,
        params,
      };

      this.pendingRequests.set(id, { resolve, reject });
      this.process.stdin.write(JSON.stringify(request) + "\n");
    });
  }

  private sendNotification(
    method: string,
    params?: Record<string, unknown>
  ): void {
    if (!this.process?.stdin?.writable) return;

    const notification = {
      jsonrpc: "2.0",
      method,
      params,
    };

    this.process.stdin.write(JSON.stringify(notification) + "\n");
  }

  private async callTool(
    name: string,
    args: Record<string, unknown>
  ): Promise<unknown> {
    if (!this.initialized) {
      await this.connect();
    }

    const result = (await this.sendRequest("tools/call", {
      name,
      arguments: args,
    })) as { content?: Array<{ type: string; text?: string }> };

    if (result?.content?.[0]?.text) {
      return result.content[0].text;
    }

    return result;
  }

  private rejectAllPending(err: Error): void {
    for (const [, pending] of this.pendingRequests) {
      pending.reject(err);
    }
    this.pendingRequests.clear();
  }

  async listStacks(): Promise<string[]> {
    try {
      const result = (await this.callTool("atmos_list_stacks", {})) as string;
      // Parse the output - format is "Available Stacks (yaml format):\n\n- stack1\n- stack2\n..."
      const lines = result.split("\n");
      return lines
        .filter((l: string) => l.startsWith("- "))
        .map((l: string) => l.substring(2).trim());
    } catch (err) {
      this.outputChannel.appendLine(
        `[MCP] listStacks failed: ${err instanceof Error ? err.message : String(err)}`
      );
      throw err;
    }
  }

  async describeComponent(component: string, stack: string): Promise<string> {
    try {
      const result = (await this.callTool("atmos_describe_component", {
        component,
        stack,
      })) as string;
      return result;
    } catch (err) {
      this.outputChannel.appendLine(
        `[MCP] describeComponent failed: ${err instanceof Error ? err.message : String(err)}`
      );
      throw err;
    }
  }

  async listComponents(): Promise<ComponentInfo[]> {
    // MCP doesn't have a dedicated list-components-with-stacks tool,
    // so we fall back to CLI for this.
    throw new Error("Not implemented via MCP - use CLI fallback");
  }

  dispose(): void {
    this.rejectAllPending(new Error("Client disposed"));
    if (this.process) {
      this.process.kill();
      this.process = undefined;
    }
    this.initialized = false;
    this.initPromise = undefined;
  }
}

/**
 * CLI-based atmos client. Executes atmos commands as individual subprocesses.
 */
class CliAtmosClient implements AtmosClient {
  constructor(
    private atmosBinary: string,
    private workspaceRoot: string,
    private outputChannel: vscode.OutputChannel
  ) {}

  private exec(args: string[]): Promise<string> {
    return new Promise((resolve, reject) => {
      execFile(
        this.atmosBinary,
        args,
        { cwd: this.workspaceRoot, maxBuffer: 10 * 1024 * 1024 },
        (err, stdout, stderr) => {
          if (err) {
            this.outputChannel.appendLine(
              `[CLI] ${args.join(" ")} failed: ${stderr || err.message}`
            );
            reject(new Error(stderr || err.message));
            return;
          }
          resolve(stdout);
        }
      );
    });
  }

  async listStacks(): Promise<string[]> {
    const output = await this.exec(["list", "stacks"]);
    return output
      .split("\n")
      .map((l) => l.trim())
      .filter((l) => l.length > 0);
  }

  async describeComponent(component: string, stack: string): Promise<string> {
    return this.exec([
      "describe",
      "component",
      component,
      "--stack",
      stack,
      "--format",
      "yaml",
    ]);
  }

  async listComponents(): Promise<ComponentInfo[]> {
    const output = await this.exec([
      "describe",
      "stacks",
      "--format",
      "json",
    ]);

    const stacks = JSON.parse(output) as Record<
      string,
      {
        components?: {
          terraform?: Record<string, unknown>;
          helmfile?: Record<string, unknown>;
        };
      }
    >;

    // Build component map: component name -> { type, stacks[] }.
    const componentMap = new Map<
      string,
      { type: string; stacks: Set<string> }
    >();

    for (const [stackName, stackData] of Object.entries(stacks)) {
      if (!stackData.components) continue;

      for (const [compType, components] of Object.entries(
        stackData.components
      )) {
        if (!components || typeof components !== "object") continue;
        for (const compName of Object.keys(components)) {
          const key = `${compType}/${compName}`;
          if (!componentMap.has(key)) {
            componentMap.set(key, { type: compType, stacks: new Set() });
          }
          componentMap.get(key)!.stacks.add(stackName);
        }
      }
    }

    return Array.from(componentMap.entries()).map(([, info]) => ({
      name: info.type.split("/").pop() || info.type,
      type: info.type,
      stacks: Array.from(info.stacks).sort(),
    }));
  }

  dispose(): void {
    // Nothing to clean up for CLI client.
  }
}

/**
 * Creates an AtmosClient with MCP as primary and CLI as fallback.
 */
export function createAtmosClient(
  atmosBinary: string,
  workspaceRoot: string,
  outputChannel: vscode.OutputChannel,
  useMcp: boolean
): AtmosClient {
  const cliClient = new CliAtmosClient(
    atmosBinary,
    workspaceRoot,
    outputChannel
  );

  if (!useMcp) {
    return cliClient;
  }

  const mcpClient = new McpAtmosClient(
    atmosBinary,
    workspaceRoot,
    outputChannel
  );

  // Return a wrapper that tries MCP first, falls back to CLI.
  return new FallbackAtmosClient(mcpClient, cliClient, outputChannel);
}

class FallbackAtmosClient implements AtmosClient {
  constructor(
    private primary: McpAtmosClient,
    private fallback: CliAtmosClient,
    private outputChannel: vscode.OutputChannel
  ) {}

  async listStacks(): Promise<string[]> {
    try {
      await this.primary.connect();
      return await this.primary.listStacks();
    } catch {
      this.outputChannel.appendLine(
        "[Atmos] MCP listStacks failed, falling back to CLI"
      );
      return this.fallback.listStacks();
    }
  }

  async describeComponent(component: string, stack: string): Promise<string> {
    try {
      await this.primary.connect();
      return await this.primary.describeComponent(component, stack);
    } catch {
      this.outputChannel.appendLine(
        "[Atmos] MCP describeComponent failed, falling back to CLI"
      );
      return this.fallback.describeComponent(component, stack);
    }
  }

  async listComponents(): Promise<ComponentInfo[]> {
    // Always use CLI for this since MCP doesn't have a combined tool.
    return this.fallback.listComponents();
  }

  dispose(): void {
    this.primary.dispose();
    this.fallback.dispose();
  }
}
