import React from 'react';
import Layout from '@theme/Layout';
import DocRootLayout from '@theme/DocRoot/Layout';
import {DocsSidebarProvider} from '@docusaurus/plugin-content-docs/client';
import type {PropSidebar} from '@docusaurus/plugin-content-docs';
import {usePluginData} from '@docusaurus/useGlobalData';
import type {FileBrowserOptions} from './types';

function DocumentationLayout({options, children}: {
  options: FileBrowserOptions;
  children: React.ReactNode;
}) {
  const {sidebar} = usePluginData('file-browser', options.id) as {
    sidebar: {name: string; items: PropSidebar};
  };
  return (
    <DocsSidebarProvider name={sidebar.name} items={sidebar.items}>
      <DocRootLayout>
        {children}
      </DocRootLayout>
    </DocsSidebarProvider>
  );
}

/** Keep reference galleries in the same desktop and mobile docs navigation. */
export default function FileBrowserLayout({options, children, ...props}: {
  options: FileBrowserOptions;
} & React.ComponentProps<typeof Layout>) {
  return (
    <Layout {...props}>
      {options.docsSidebar
        ? <DocumentationLayout options={options}>{children}</DocumentationLayout>
        : children}
    </Layout>
  );
}
