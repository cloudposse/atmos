/**
 * TypeScript types for the FileBrowser component.
 */

/**
 * Represents a file in the tree.
 */
export interface FileNode {
  name: string;
  type: 'file';
  path: string;
  relativePath: string;
  extension: string;
  language: string;
  size: number;
  content: string | null;
  githubUrl: string | null;
}

/**
 * Represents a directory in the tree.
 */
export interface DirectoryNode {
  name: string;
  type: 'directory';
  path: string;
  relativePath: string;
  children: TreeNode[];
  readme: FileNode | null;
  fileCount: number;
  githubUrl: string | null;
}

/**
 * Union type for tree nodes.
 */
export type TreeNode = FileNode | DirectoryNode;

/**
 * Represents an example project.
 */
export interface ExampleProject {
  name: string;
  path: string;
  description: string;
  hasReadme: boolean;
  hasAtmosYaml: boolean;
  tags: string[];
  root: DirectoryNode;
}

/**
 * Root data structure for the examples tree.
 */
export interface ExamplesTree {
  examples: ExampleProject[];
  tags: string[];
  generatedAt: string;
  totalFiles: number;
  totalExamples: number;
}

/**
 * Plugin options passed to components.
 */
export interface FileBrowserOptions {
  routeBasePath: string;
  title: string;
  description: string;
  githubRepo: string;
  githubBranch: string;
  githubPath: string;
}

/**
 * Type guard to check if a node is a FileNode.
 */
export function isFileNode(node: TreeNode): node is FileNode {
  return node.type === 'file';
}

/**
 * Type guard to check if a node is a DirectoryNode.
 */
export function isDirectoryNode(node: TreeNode): node is DirectoryNode {
  return node.type === 'directory';
}
