import * as vscode from "vscode";
import { AtmosClient, ComponentInfo } from "./atmos-client";

type TreeItem = ComponentTypeNode | ComponentNode | StackInstanceNode;

class ComponentTypeNode extends vscode.TreeItem {
  constructor(public readonly componentType: string) {
    super(componentType, vscode.TreeItemCollapsibleState.Expanded);
    this.contextValue = "componentType";
    this.iconPath = new vscode.ThemeIcon("symbol-folder");
  }
}

class ComponentNode extends vscode.TreeItem {
  constructor(
    public readonly componentName: string,
    public readonly componentType: string,
    public readonly stackCount: number
  ) {
    super(componentName, vscode.TreeItemCollapsibleState.Collapsed);
    this.contextValue = "component";
    this.description = `${stackCount} stack${stackCount === 1 ? "" : "s"}`;
    this.iconPath = new vscode.ThemeIcon("symbol-module");
  }
}

class StackInstanceNode extends vscode.TreeItem {
  constructor(
    public readonly stackName: string,
    public readonly componentName: string,
    public readonly componentType: string
  ) {
    super(stackName, vscode.TreeItemCollapsibleState.None);
    this.contextValue = "stackInstance";
    this.description = "";
    this.iconPath = new vscode.ThemeIcon("layers");
    this.command = {
      command: "atmos.showRenderedYaml",
      title: "Show Rendered YAML",
      arguments: [componentName, stackName],
    };
    this.tooltip = `${componentType}/${componentName} in ${stackName}`;
  }
}

export class ComponentTreeProvider
  implements vscode.TreeDataProvider<TreeItem>
{
  private _onDidChangeTreeData = new vscode.EventEmitter<
    TreeItem | undefined | null | void
  >();
  readonly onDidChangeTreeData = this._onDidChangeTreeData.event;

  private components: ComponentInfo[] = [];
  private loading = false;

  constructor(private client: AtmosClient) {}

  refresh(): void {
    this.components = [];
    this._onDidChangeTreeData.fire();
  }

  getTreeItem(element: TreeItem): vscode.TreeItem {
    return element;
  }

  async getChildren(element?: TreeItem): Promise<TreeItem[]> {
    if (!element) {
      return this.getRootNodes();
    }

    if (element instanceof ComponentTypeNode) {
      return this.getComponentNodes(element.componentType);
    }

    if (element instanceof ComponentNode) {
      return this.getStackNodes(element.componentName, element.componentType);
    }

    return [];
  }

  private async getRootNodes(): Promise<TreeItem[]> {
    if (this.components.length === 0 && !this.loading) {
      await this.loadComponents();
    }

    // Group by component type.
    const types = new Set<string>();
    for (const comp of this.components) {
      types.add(comp.type);
    }

    return Array.from(types)
      .sort()
      .map((t) => new ComponentTypeNode(t));
  }

  private getComponentNodes(type: string): TreeItem[] {
    const typeComponents = this.components.filter((c) => c.type === type);

    return typeComponents
      .sort((a, b) => a.name.localeCompare(b.name))
      .map((c) => new ComponentNode(c.name, c.type, c.stacks.length));
  }

  private getStackNodes(componentName: string, componentType: string): TreeItem[] {
    const comp = this.components.find(
      (c) => c.name === componentName && c.type === componentType
    );
    if (!comp) return [];

    return comp.stacks
      .sort()
      .map((s) => new StackInstanceNode(s, componentName, componentType));
  }

  private async loadComponents(): Promise<void> {
    this.loading = true;
    try {
      this.components = await this.client.listComponents();
    } catch (err) {
      const message =
        err instanceof Error ? err.message : String(err);
      vscode.window.showWarningMessage(
        `Failed to load Atmos components: ${message}`
      );
      this.components = [];
    } finally {
      this.loading = false;
    }
  }

  dispose(): void {
    this._onDidChangeTreeData.dispose();
  }
}
