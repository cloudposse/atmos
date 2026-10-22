import React from 'react';
import Markdown from 'react-markdown';
import Heading from '@theme/Heading';
import Link from '@docusaurus/Link';
import { availabilityMarkdown } from './availability.mjs';

const components = {
  h2: ({ children }) => <Heading as="h2" id="availability-and-evaluation">{children}</Heading>,
  a: ({ href, children }) => <Link to={href}>{children}</Link>,
};

export default function YamlFunctionAvailability({ name }: { name: string }): JSX.Element {
  return <Markdown components={components}>{availabilityMarkdown(name)}</Markdown>;
}
