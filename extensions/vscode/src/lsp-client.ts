import * as vscode from "vscode";
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
} from "vscode-languageclient/node";

let client: LanguageClient | undefined;

export function createLspClient(
  context: vscode.ExtensionContext,
  atmosBinary: string
): LanguageClient {
  const serverOptions: ServerOptions = {
    command: atmosBinary,
    args: ["lsp", "start", "--transport", "stdio"],
  };

  const clientOptions: LanguageClientOptions = {
    documentSelector: [
      {
        scheme: "file",
        language: "yaml",
        pattern: "**/stacks/**/*.{yaml,yml}",
      },
      { scheme: "file", language: "yaml", pattern: "**/atmos.yaml" },
    ],
    synchronize: {
      fileEvents: vscode.workspace.createFileSystemWatcher("**/*.{yaml,yml}"),
    },
    outputChannelName: "Atmos LSP",
  };

  client = new LanguageClient(
    "atmosLsp",
    "Atmos LSP Server",
    serverOptions,
    clientOptions
  );

  return client;
}

export async function startLspClient(lspClient: LanguageClient): Promise<void> {
  await lspClient.start();
}

export async function stopLspClient(): Promise<void> {
  if (client) {
    await client.stop();
    client = undefined;
  }
}
