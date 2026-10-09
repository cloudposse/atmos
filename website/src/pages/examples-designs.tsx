import React from "react";
import ExampleCatalogPage from "../components/FileBrowser/ExampleCatalogPage";
import type {
  ExamplesTree,
  FileBrowserOptions,
} from "../components/FileBrowser/types";
import treeData from "@generated/file-browser/examples/file-browser-tree-examples.json";
import optionsData from "@generated/file-browser/examples/file-browser-options-examples.json";

export default function ExamplesDesigns() {
  return (
    <ExampleCatalogPage
      tree={treeData as ExamplesTree}
      options={optionsData as FileBrowserOptions}
      preview
    />
  );
}
