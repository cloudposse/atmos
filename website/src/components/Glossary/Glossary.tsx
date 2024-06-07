import React, { useEffect, useState } from 'react';
import BrowserOnly from '@docusaurus/BrowserOnly';
import { useBaseUrlUtils } from '@docusaurus/useBaseUrl';
import Link from '@docusaurus/Link';
import { marked } from 'marked';

const Glossary = () => {
  const [content, setContent] = useState();
  const { withBaseUrl } = useBaseUrlUtils();

  useEffect(() => {
    if (typeof window !== undefined) {
      const JSONurl = withBaseUrl('docs/glossary.json');
      if (!content) {
        if (!window._cachedGlossary) {
          fetch(JSONurl)
            .then(res => res.json())
            .then(obj => {
              setContent(obj);
              window._cachedGlossary = obj;
            });
        } else {
          setContent(window._cachedGlossary);
        }
      }
    }
  }, [content])

  // Use content[key].content, for the body content. However, it's raw and there's no way to render it as markdown or MDX
  return (
    <BrowserOnly
      fallback={<div>Failed to render glossary.</div>}>
      {() =>
        <dl>{content ?
          <>
            {
            Object.keys(content).sort().map(key => {
                return (
                    <p key={key}>
                        <dt><Link to={withBaseUrl(content[key].metadata.slug || key)}>{content[key].metadata.title}</Link></dt>
                        <dd dangerouslySetInnerHTML={{ __html: marked(content[key].metadata.hoverText) }}/>
                        {content[key].metadata.disambiguation && Object.entries(content[key].metadata.disambiguation).length > 0 && (
                            <dd class="disambiguation">
                                <label>See also:</label>
                                {Object.entries(content[key].metadata.disambiguation).map(([term, desc]) => {
                                    return (
                                        <Link to={withBaseUrl('terms/' + term)}>{desc}</Link>
                                    )       
                                })}
                            </dd>
                        )}
                    </p>
                )
            })
            }
          </> :
          'loading...'}
        </dl>
      }
    </BrowserOnly>
  );
};

export default Glossary;
