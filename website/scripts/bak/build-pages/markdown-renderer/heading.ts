import {createDocument} from '@stencil/core/mock-doc';

import {slugify} from '../../../src/utils';

export default (text: string, level: number) => {
  // TODO: use mockDoc createFragment() once it's exported
  const doc = createDocument(text);
  const {textContent} = doc.body;

  let hash;
  if (textContent !== null) {
    hash = slugify(textContent);
  }

  return `
    <h${level} id="${hash}">
      <a href="#${hash}">${text}</a>
    </h${level}>
  `;
};
