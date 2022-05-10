import React from 'react';

import {useScript} from '@site/src/utils/hooks';

function CodePen(props): JSX.Element {
  const status = useScript('https://static.codepen.io/assets/embed/ei.js');
  // console.log('test',status, props)
  return (
    <div
      className="codepen"
      data-height={props.height}
      data-theme-id={props.theme}
      data-default-tab={props.defaultTab}
      data-user={props.user}
      data-slug-hash={props.slug}
      data-preview={props.preview ? 'true' : 'false'}
      data-pen-title={props.penTitle}
      no-prerender="true"
    ></div>
  );
}

export default CodePen;
