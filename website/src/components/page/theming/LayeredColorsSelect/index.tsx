import CodeColor from '@components/page/theming/CodeColor';
import React, { useEffect, useRef, useState } from 'react';

import styles from './styles.module.scss';
import ColorDot from '../ColorDot';

import InputWrapper from '../InputWrapper';

import useThemeContext from '@theme/hooks/useThemeContext';
import clsx from 'clsx';

export default function LayeredColorsSelect({ ...props }) {
  const { isDarkTheme } = useThemeContext();

  const [color, setColor] = useState('primary');
  const el = useRef<HTMLDivElement>(null);

  const [variations, setVariations] = useState([]);

  useEffect(() => {
    setVariations([
      {
        property: `--ion-color-${color}`,
        name: 'Base',
        description: 'The main color that all variations are derived from',
        value: getComputedStyle(el.current).getPropertyValue(`--ion-color-${color}`),
      },
      {
        property: `--ion-color-${color}-rgb`,
        name: 'Base (rgb)',
        rgb: true,
        description: 'The base color in red, green, blue format',
        value: getComputedStyle(el.current).getPropertyValue(`--ion-color-${color}-rgb`),
      },
      {
        property: `--ion-color-${color}-contrast`,
        name: 'Contrast',
        description: 'The opposite of the base color, should be visible against the base color',
        value: getComputedStyle(el.current).getPropertyValue(`--ion-color-${color}-contrast`),
      },
      {
        property: `--ion-color-${color}-contrast-rgb`,
        name: 'Contrast (rgb)',
        rgb: true,
        description: 'The contrast color in red, green, blue format',
        value: getComputedStyle(el.current).getPropertyValue(`--ion-color-${color}-contrast-rgb`),
      },
      {
        property: `--ion-color-${color}-shade`,
        name: 'Shade',
        description: 'A slightly darker version of the base color',
        value: getComputedStyle(el.current).getPropertyValue(`--ion-color-${color}-shade`),
      },
      {
        property: `--ion-color-${color}-tint`,
        name: 'Tint',
        description: 'A slightly lighter version of the base color',
        value: getComputedStyle(el.current).getPropertyValue(`--ion-color-${color}-tint`),
      },
    ]);
  }, [color]);

  return (
    <div
      {...props}
      ref={el}
      className={clsx(styles.layeredColorsSelect, styles[`layeredColorsSelect${isDarkTheme ? 'Dark' : 'Light'}`])}
    >
      <div className={styles.selectRow}>
        <ColorDot color={`var(--ion-color-${color})`} />
        <InputWrapper>
          <select value={color} onChange={(ev) => setColor((ev.target as HTMLSelectElement).value)}>
            <option value="primary">Primary</option>
            <option value="secondary">Secondary</option>
            <option value="tertiary">Tertiary</option>
            <option value="success">Success</option>
            <option value="warning">Warning</option>
            <option value="danger">Danger</option>
            <option value="dark">Dark</option>
            <option value="medium">Medium</option>
            <option value="light">Light</option>
          </select>
        </InputWrapper>
      </div>
      <table>
        <tr>
          <th>Name</th>
          <th>Property</th>
          <th>Default Value</th>
          <th>Description</th>
        </tr>
        {variations.map((variation) => {
          const codeColor = variation.rgb ? `rgb(${variation.value})` : `${variation.value}`;

          return (
            <tr>
              <td className={styles.colorName}>{variation.name}</td>
              <td className={styles.colorProperty}>
                <code>{variation.property}</code>
              </td>
              <td className={styles.colorValue}>
                <CodeColor color={codeColor}>{variation.value}</CodeColor>
              </td>
              <td className={styles.colorDescription}>{variation.description}</td>
            </tr>
          );
        })}
      </table>
    </div>
  );
}
