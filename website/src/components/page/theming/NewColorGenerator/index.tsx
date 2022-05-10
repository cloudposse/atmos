import React from 'react';
import CodeColor from '@components/page/theming/CodeColor';
import { useState } from 'react';
import { generateColor } from '../_utils/index';
import { useEffect } from 'react';

import styles from './index.module.scss';
import InputWrapper from '../InputWrapper';
import ColorInput from '../ColorInput';
import ColorDot from '../ColorDot';

export default function ColorGenerator() {
  const [name, setName] = useState('New');
  const [value, setValue] = useState('#69bb7b');
  const [color, setColor] = useState(generateColor(value));

  useEffect(() => {
    setColor(generateColor(value));
  }, [value]);

  const nameLower = name.trim().toLowerCase();
  const { value: colorValue, valueRgb, contrast, contrastRgb, tint, shade } = color;

  return (
    <div className={styles.newColorGenerator}>
      <div className={styles.top}>
        <div className={styles.top__start}>
          <ColorDot color={value} />
          <InputWrapper>
            <input onChange={({ target }) => setName(target.value)} value={name} />
          </InputWrapper>
        </div>
        <ColorInput color={value} setColor={setValue} className={styles.top__end} />
      </div>
      <pre className={styles.codePre}>
        <code>
          :root {'{'}
          {'\n'}
          {'\t'}--ion-color-{nameLower}: <CodeColor color={colorValue}>{colorValue}</CodeColor>;{'\n'}
          {'\t'}--ion-color-{nameLower}-rgb: <CodeColor color={colorValue}>{valueRgb}</CodeColor>;{'\n'}
          {'\t'}--ion-color-{nameLower}-contrast: <CodeColor color={contrast}>{contrast}</CodeColor>;{'\n'}
          {'\t'}--ion-color-{nameLower}-contrast-rgb: <CodeColor color={contrast}>{contrastRgb}</CodeColor>;{'\n'}
          {'\t'}--ion-color-{nameLower}-shade: <CodeColor color={shade}>{shade}</CodeColor>;{'\n'}
          {'\t'}--ion-color-{nameLower}-tint: <CodeColor color={tint}>{tint}</CodeColor>;{'\n'}
          {'}'}
          {'\n'}
          {'\n'}
          .ion-color-{nameLower} {'{'}
          {'\n'}
          {'\t'}--ion-color-base: var(--ion-color-{nameLower});{'\n'}
          {'\t'}--ion-color-base-rgb: var(--ion-color-{nameLower}-rgb);{'\n'}
          {'\t'}--ion-color-contrast: var(--ion-color-{nameLower}-contrast);{'\n'}
          {'\t'}--ion-color-contrast-rgb: var(--ion-color-{nameLower}-contrast-rgb);{'\n'}
          {'\t'}--ion-color-shade: var(--ion-color-{nameLower}-shade);{'\n'}
          {'\t'}--ion-color-tint: var(--ion-color-{nameLower}-tint);{'\n'}
          {'}'}
        </code>
      </pre>
    </div>
  );
}
