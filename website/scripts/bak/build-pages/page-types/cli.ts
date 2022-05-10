import {commands} from '../../data/cli.json';
import {buildPages, Page} from '../index';
import renderMarkdown from '../markdown-renderer';

export default {
  title: 'Build CLI pages',
  task: () => buildPages(getCLIPages)
};

const getCLIPages = async (): Promise<Page[]> => {
  return commands.map((command: { [x: string]: any; name: any; description: any; summary: any; inputs: any; options: any; }) => {
    const {name, description, summary, inputs, options, ...rest} = command;

    const shortName = name.slice(6).replace(/\s/g, '-');

    return {
      title: name,
      body: renderMarkdown(description),
      path: `/docs/cli/commands/${shortName}`,
      summary: renderMarkdown(summary),
      inputs: renderInputs(inputs),
      options: renderOptions(options),
      skipIntros: true,
      template: 'cli',
      meta: {
        title: `${name}: ${summary}`,
        description: summary
      },
      ...rest
    };
  });
};

const renderInputs = (inputs: any) => inputs.map((input: any) => ({
  ...input,
  summary: renderMarkdown(input.summary),
}));

const renderOptions = (options: any) => options.map((option: any) => ({
  ...option,
  summary: renderMarkdown(option.summary),
}));
