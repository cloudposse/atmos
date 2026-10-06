import React from 'react';
import Link from '@docusaurus/Link';
import DocCardList from '@theme/DocCardList';
import {useCurrentSidebarCategory} from '@docusaurus/plugin-content-docs/client';

type ComponentType = {id: string; native: boolean; implementation: string; description: string};

/** The generated sidebar carries the same front matter used by this overview. */
export default function ComponentLibraryTypes(): JSX.Element {
  const {items} = useCurrentSidebarCategory();
  const types = items.flatMap(item => {
    const component = item.customProps?.componentType as ComponentType | undefined;
    return item.type !== 'html' && item.href && component?.native
      ? [{...component, href: item.href, label: item.label}]
      : [];
  });
  return (
    <>
      <p>Atmos natively supports {types.length} component types:</p>
      <table>
        <thead>
          <tr><th>Type</th><th>Implementation</th><th>Description</th></tr>
        </thead>
        <tbody>
          {types.map(item => (
            <tr key={item.id}>
              <td style={{whiteSpace: 'nowrap'}}><Link to={item.href}>{item.label}</Link></td>
              <td>{item.implementation}</td>
              <td>{item.description}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
}

export function ComponentLibraryCards(): JSX.Element {
  const {items} = useCurrentSidebarCategory();
  return <DocCardList items={items} />;
}
