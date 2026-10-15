import React from "react";
import Layout from "@theme/Layout";
import DocRootLayout from "@theme/DocRoot/Layout";
import { DocsSidebarProvider } from "@docusaurus/plugin-content-docs/client";
import type { PropSidebar } from "@docusaurus/plugin-content-docs";
import { usePluginData } from "@docusaurus/useGlobalData";
import type { FileBrowserOptions } from "./types";

type SidebarTransform = (items: PropSidebar) => PropSidebar;

function DocumentationLayout({
  options,
  children,
  sidebarTransform,
}: {
  options: FileBrowserOptions;
  children: React.ReactNode;
  sidebarTransform?: SidebarTransform;
}) {
  const { sidebar } = usePluginData("file-browser", options.id) as {
    sidebar: { name: string; items: PropSidebar };
  };
  return (
    <DocsSidebarProvider
      name={sidebar.name}
      items={sidebarTransform ? sidebarTransform(sidebar.items) : sidebar.items}
    >
      <DocRootLayout>{children}</DocRootLayout>
    </DocsSidebarProvider>
  );
}

/** Keep reference galleries in the same desktop and mobile docs navigation. */
export default function FileBrowserLayout({
  options,
  children,
  sidebarTransform,
  ...props
}: {
  options: FileBrowserOptions;
  sidebarTransform?: SidebarTransform;
} & React.ComponentProps<typeof Layout>) {
  return (
    <Layout {...props}>
      {options.docsSidebar ? (
        <DocumentationLayout
          options={options}
          sidebarTransform={sidebarTransform}
        >
          {children}
        </DocumentationLayout>
      ) : (
        children
      )}
    </Layout>
  );
}
