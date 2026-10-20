import React from 'react';
import Link from '@docusaurus/Link';
import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';
import CodeBlock from '@theme/CodeBlock';
import InstallWidget from '@site/src/components/InstallWidget';
import styles from './styles.module.css';

interface TryExampleProps {
  /** Project path relative to examples/, for example "cloudformation". */
  example: string;
  /** Short, human-readable tab label, especially when grouping examples. */
  label?: string;
  /** Hide the link back to the example when already on its detail page. */
  showExampleLink?: boolean;
  /** Prerequisites and next steps shown in the example tab. */
  children?: React.ReactNode;
}

export default function TryExample(props: TryExampleProps) {
  return <ExampleTabs examples={[props]} />;
}

/** Group related examples behind one installation tab. Use direct TryExample children. */
export function TryExamples({ children }: { children: React.ReactNode }) {
  const examples = React.Children.toArray(children)
    .filter((child): child is React.ReactElement<TryExampleProps> =>
      React.isValidElement<TryExampleProps>(child) && typeof child.props.example === 'string')
    .map((child) => child.props);

  return examples.length ? <ExampleTabs examples={examples} /> : null;
}

function ExampleTabs({ examples }: { examples: TryExampleProps[] }) {
  const single = examples.length === 1;
  const first = examples[0];

  return (
    <div className={styles.example}>
      <Tabs defaultValue={`example:${first.example}`}>
        <TabItem value="install" label="Install Atmos">
          <InstallWidget variant="docs" />
          <div className={styles.links}>
            <Link to="/install">Installation guide →</Link>
            {single && first.showExampleLink !== false && (
              <Link to={`/examples/${first.example}`}>Explore the example →</Link>
            )}
          </div>
        </TabItem>
        {examples.map(({ example, label, children, showExampleLink = true }) => (
          <TabItem
            key={example}
            value={`example:${example}`}
            label={label || (single ? 'Try the example' : example)}
          >
            <p>Copy the example into a new directory and make it your own:</p>
            <CodeBlock language="shell">{`atmos init examples/${example}\ncd ${example.split('/').at(-1)}`}</CodeBlock>
            {children}
            <div className={styles.links}>
              {showExampleLink && <Link to={`/examples/${example}`}>Explore the example →</Link>}
              <Link to="/cli/commands/init#examples-and-source-directories">Initialization and repository settings</Link>
            </div>
          </TabItem>
        ))}
      </Tabs>
    </div>
  );
}
