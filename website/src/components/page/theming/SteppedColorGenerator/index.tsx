import React, {useEffect, useState} from 'react';
import CodeColor from '../CodeColor';

import ColorDot from '../ColorDot';
import ColorInput from '../ColorInput';

import {Color} from '../_utils/color';

import {generateSteppedColors} from '../_utils/index';
import clsx from 'clsx';
import styles from './index.module.scss';

export default function ColorGenerator(props) {
  const [backgroundColor, setBackgroundColor] = useState('#ffffff');
  const [textColor, setTextColor] = useState('#000000');

  const [steppedColors, setSteppedColors] = useState(generateSteppedColors(backgroundColor, textColor));

  useEffect(() => {
    setSteppedColors(generateSteppedColors(backgroundColor, textColor));
  }, [backgroundColor, textColor]);

  return (
    <div className={clsx(props.className, 'stepped-color-generator')}>
      <div className={clsx(styles.inputRows)}>
        <ColorDot color={backgroundColor}/>
        <h3>Background</h3>
        <ColorInput color={backgroundColor} setColor={setBackgroundColor}/>
        <ColorDot color={textColor}/>
        <h3>Text</h3>
        <ColorInput color={textColor} setColor={setTextColor}/>
      </div>
      <pre className={clsx(styles.codePre)}>
        <code>
          :root {'{'}
          {'\n'}
          {'\t'}--ion-background-color: <CodeColor color={backgroundColor}>{backgroundColor}</CodeColor>;{'\n'}
          {'\t'}--ion-background-color-rgb:{' '}
          <CodeColor color={backgroundColor}>{new Color(backgroundColor).toList()}</CodeColor>;{'\n'}
          {'\n'}
          {'\t'}--ion-text-color: <CodeColor color={textColor}>{textColor}</CodeColor>;{'\n'}
          {'\t'}--ion-text-color-rgb: <CodeColor color={textColor}>{new Color(textColor).toList()}</CodeColor>;{'\n'}
          {'\n'}
          {steppedColors.map((color, i) => (
            <>
              {'\t'}--ion-color-step-{(i + 1) * 50}: <CodeColor color={color}>{color}</CodeColor>;{'\n'}
            </>
          ))}
          {'}'}
          {'\n'}
        </code>
      </pre>
    </div>
  );
}
