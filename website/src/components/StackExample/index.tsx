import React, { useState } from 'react';
import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';
import CodeBlock from '@theme/CodeBlock';
import './styles.css';

interface StackExampleProps {
  yaml: string;
  json: string;
  hcl: string;
  title?: string;
}

/**
 * StackExample component that displays configuration in multiple formats.
 *
 * Renders tabbed code blocks showing equivalent YAML, JSON, and HCL syntax
 * for Atmos stack configurations. Function syntax is automatically translated
 * between formats.
 *
 * Usage in MDX (via remark-stack-example plugin):
 * ```yaml stack-example
 * settings:
 *   region: !env AWS_REGION
 * ```
 *
 * Or direct component usage:
 * <StackExample
 *   yaml="settings:\n  region: !env AWS_REGION"
 *   json='{"settings": {"region": "${env:AWS_REGION}"}}'
 *   hcl='settings = {\n  region = atmos_env("AWS_REGION")\n}'
 * />
 */
const StackExample: React.FC<StackExampleProps> = ({ yaml, json, hcl, title }) => {
  const [copied, setCopied] = useState<string | null>(null);

  const handleCopy = async (format: string, content: string) => {
    try {
      await navigator.clipboard.writeText(content);
      setCopied(format);
      setTimeout(() => setCopied(null), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  };

  return (
    <div className="stack-example">
      {title && <div className="stack-example__title">{title}</div>}
      <Tabs queryString="stack-format" defaultValue="yaml">
        <TabItem value="yaml" label="YAML">
          <div className="stack-example__code">
            <CodeBlock language="yaml">{yaml}</CodeBlock>
          </div>
        </TabItem>
        <TabItem value="json" label="JSON">
          <div className="stack-example__code">
            <CodeBlock language="json">{json}</CodeBlock>
          </div>
        </TabItem>
        <TabItem value="hcl" label="HCL">
          <div className="stack-example__code">
            <CodeBlock language="hcl">{hcl}</CodeBlock>
          </div>
        </TabItem>
      </Tabs>
    </div>
  );
};

export default StackExample;
