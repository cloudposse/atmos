import * as vscode from "vscode";
import * as fs from "fs";
import * as path from "path";
import { createLspClient, startLspClient, stopLspClient } from "./lsp-client";
import { AtmosClient, createAtmosClient } from "./atmos-client";
import { ComponentTreeProvider } from "./component-tree";
import {
  RenderedYamlProvider,
  RENDERED_YAML_SCHEME,
  showRenderedYaml,
} from "./rendered-yaml-provider";
import { LanguageClient } from "vscode-languageclient/node";

let lspClient: LanguageClient | undefined;
let atmosClient: AtmosClient | undefined;
let treeProvider: ComponentTreeProvider | undefined;
let yamlProvider: RenderedYamlProvider | undefined;
let outputChannel: vscode.OutputChannel;

export async function activate(
  context: vscode.ExtensionContext
): Promise<void> {
  outputChannel = vscode.window.createOutputChannel("Atmos");
  context.subscriptions.push(outputChannel);

  // Check if this is an Atmos project.
  const workspaceRoot = getWorkspaceRoot();
  if (!workspaceRoot) {
    outputChannel.appendLine("No workspace folder found");
    return;
  }

  const atmosYamlPath = path.join(workspaceRoot, "atmos.yaml");
  if (!fs.existsSync(atmosYamlPath)) {
    outputChannel.appendLine(
      "No atmos.yaml found in workspace root, extension inactive"
    );
    return;
  }

  const config = vscode.workspace.getConfiguration("atmos");
  const atmosBinary = config.get<string>("binaryPath", "atmos");
  const lspEnabled = config.get<boolean>("lsp.enabled", true);
  const mcpEnabled = config.get<boolean>("mcp.enabled", true);

  outputChannel.appendLine(`Atmos extension activating in ${workspaceRoot}`);
  outputChannel.appendLine(`Binary: ${atmosBinary}`);

  // Start LSP client for completion, hover, diagnostics, go-to-definition.
  if (lspEnabled) {
    try {
      lspClient = createLspClient(context, atmosBinary);
      await startLspClient(lspClient);
      outputChannel.appendLine("LSP client started");
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      outputChannel.appendLine(`Failed to start LSP client: ${message}`);
      vscode.window.showWarningMessage(
        `Atmos LSP failed to start: ${message}`
      );
    }
  }

  // Create atmos client for component data.
  atmosClient = createAtmosClient(
    atmosBinary,
    workspaceRoot,
    outputChannel,
    mcpEnabled
  );

  // Register rendered YAML virtual document provider.
  yamlProvider = new RenderedYamlProvider(atmosClient);
  context.subscriptions.push(
    vscode.workspace.registerTextDocumentContentProvider(
      RENDERED_YAML_SCHEME,
      yamlProvider
    )
  );

  // Register component tree view.
  treeProvider = new ComponentTreeProvider(atmosClient);
  const treeView = vscode.window.createTreeView("atmosComponents", {
    treeDataProvider: treeProvider,
    showCollapseAll: true,
  });
  context.subscriptions.push(treeView);

  // Register commands.
  context.subscriptions.push(
    vscode.commands.registerCommand(
      "atmos.showRenderedYaml",
      async (component?: string, stack?: string) => {
        if (!component || !stack) {
          // Prompt user to select component and stack.
          const selection = await promptComponentStack();
          if (!selection) return;
          component = selection.component;
          stack = selection.stack;
        }
        await showRenderedYaml(component, stack);
      }
    )
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("atmos.refreshComponents", () => {
      treeProvider?.refresh();
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand(
      "atmos.describeComponent",
      async (component?: string, stack?: string) => {
        if (!component || !stack) {
          const selection = await promptComponentStack();
          if (!selection) return;
          component = selection.component;
          stack = selection.stack;
        }
        await showRenderedYaml(component, stack);
      }
    )
  );

  outputChannel.appendLine("Atmos extension activated");
}

export async function deactivate(): Promise<void> {
  await stopLspClient();
  atmosClient?.dispose();
  treeProvider?.dispose();
  yamlProvider?.dispose();
}

function getWorkspaceRoot(): string | undefined {
  const folders = vscode.workspace.workspaceFolders;
  if (!folders || folders.length === 0) {
    return undefined;
  }
  return folders[0].uri.fsPath;
}

async function promptComponentStack(): Promise<
  { component: string; stack: string } | undefined
> {
  if (!atmosClient) {
    vscode.window.showErrorMessage("Atmos client not initialized");
    return undefined;
  }

  try {
    const stacks = await atmosClient.listStacks();
    const stack = await vscode.window.showQuickPick(stacks, {
      placeHolder: "Select a stack",
      title: "Atmos: Select Stack",
    });
    if (!stack) return undefined;

    const component = await vscode.window.showInputBox({
      placeHolder: "Enter component name (e.g., vpc, rds)",
      title: "Atmos: Enter Component Name",
      validateInput: (value) => {
        if (!value.trim()) return "Component name is required";
        return undefined;
      },
    });
    if (!component) return undefined;

    return { component: component.trim(), stack };
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    vscode.window.showErrorMessage(
      `Failed to query Atmos: ${message}`
    );
    return undefined;
  }
}
