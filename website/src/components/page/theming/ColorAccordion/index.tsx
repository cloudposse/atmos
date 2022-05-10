import clsx from 'clsx';
import React, { useCallback, useEffect, useRef, useState } from 'react';

import styles from './styles.module.css';

export default function ColorAccordion({ ...props }) {
  const [colors, setColors] = useState([]);

  const [activeColor, setActiveColor] = useState('');

  const el = useRef<HTMLUListElement>(null);

  useEffect(() => {
    setColors(['primary', 'secondary', 'tertiary', 'success', 'warning', 'danger', 'dark', 'medium', 'light']);
  }, []);

  const getColors = useCallback(
    (color) => ({
      baseColor: getComputedStyle(el.current).getPropertyValue(`--ion-color-${color}`),
      shadeColor: getComputedStyle(el.current).getPropertyValue(`--ion-color-${color}-shade`),
      tintColor: getComputedStyle(el.current).getPropertyValue(`--ion-color-${color}-tint`),
    }),
    []
  );

  return (
    <ul
      {...props}
      ref={el}
      className={clsx({
        [styles.colorAccordion]: true,
        [props.className]: Boolean(props.className),
        'color-accordion': true,
      })}
    >
      {colors.map((color) => {
        const { baseColor, shadeColor, tintColor } = getColors(color);

        return (
          <li
            className={clsx({
              [styles.colorMenuItem]: true,
              [styles.colorMenuItemActive]: color === activeColor,
            })}
            style={
              {
                'background-color': `var(--ion-color-${color})`,
                color: `var(--ion-color-${color}-contrast)`,
              } as any
            }
          >
            <div className={styles.colorMenuText} onClick={() => setActiveColor(activeColor === color ? '' : color)}>
              {color[0].toUpperCase() + color.substr(1)}
              <div className={styles.colorMenuValue}>{baseColor}</div>
            </div>

            <svg width="10px" height="6px" viewBox="0 0 10 6" version="1.1" xmlns="http://www.w3.org/2000/svg">
              <g
                id="Welcome"
                stroke="none"
                stroke-width="1"
                fill="none"
                fill-rule="evenodd"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <g
                  id="Desktop-HD"
                  transform="translate(-1025.000000, -335.000000)"
                  stroke="currentColor"
                  stroke-width="2"
                >
                  <polyline
                    id="arrow"
                    transform="translate(1030.000000, 338.000000) rotate(90.000000) translate(-1030.000000, -338.000000) "
                    points="1028 334 1032 338.020022 1028 342"
                  ></polyline>
                </g>
              </g>
            </svg>

            <ul className={styles.colorSubmenu}>
              <li
                className={styles.colorSubmenuItem}
                style={
                  {
                    'background-color': `var(--ion-color-${color}-shade)`,
                    color: `var(--ion-color-${color}-contrast)`,
                  } as any
                }
              >
                <div className={styles.colorMenuText}>
                  Shade
                  <div className={styles.colorMenuValue}>{shadeColor}</div>
                </div>
              </li>
              <li
                className={styles.colorSubmenuItem}
                style={
                  {
                    'background-color': `var(--ion-color-${color}-tint)`,
                    color: `var(--ion-color-${color}-contrast)`,
                  } as any
                }
              >
                <div className={styles.colorMenuText}>
                  Tint
                  <div className={styles.colorMenuValue}>{tintColor}</div>
                </div>
              </li>
            </ul>
          </li>
        );
      })}
    </ul>
  );
}
