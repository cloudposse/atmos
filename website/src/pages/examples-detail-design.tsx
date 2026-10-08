import React from "react";
import ExampleDetailPage from "../components/FileBrowser/ExampleDetailPage";
import type {
  ExamplesTree,
  FileBrowserOptions,
} from "../components/FileBrowser/types";
import treeData from "@generated/file-browser/examples/file-browser-tree-examples.json";
import optionsData from "@generated/file-browser/examples/file-browser-options-examples.json";

export default function ExampleDetailDesign() {
  const example = (treeData as ExamplesTree).examples.find(
    (item) => item.name === "quick-start-simple",
  )!;
  return (
    <ExampleDetailPage
      example={example}
      options={optionsData as FileBrowserOptions}
      preview
    />
  );
}
