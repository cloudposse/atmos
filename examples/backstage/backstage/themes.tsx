import type {} from '@backstage/core-components';
import type {} from '@backstage/plugin-catalog';
import type {} from '@backstage/plugin-catalog-react';

// These named style slots exist in CatalogAutocomplete but are not yet exported
// in catalog-react's theme declarations.
declare module '@backstage/theme' {
  interface OverrideComponentNameToClassKeys {
    BackstageAutocomplete: 'root' | 'label';
    BackstageAutocompleteBase: 'inputRoot' | 'input' | 'popupIndicator';
  }
}
import { PropsWithChildren } from 'react';
import {
  createUnifiedTheme,
  defaultTypography,
  palettes,
  UnifiedThemeProvider,
} from '@backstage/theme';
import LightIcon from '@material-ui/icons/WbSunny';
import DarkIcon from '@material-ui/icons/Brightness2';
import '@fontsource-variable/geist';
import '@fontsource-variable/geist-mono';

const fontFamily = '"Geist Variable", -apple-system, BlinkMacSystemFont, sans-serif';
const mono = '"Geist Mono Variable", ui-monospace, monospace';

function createGeistTheme(mode: 'light' | 'dark') {
  const dark = mode === 'dark';
  const background = dark ? '#0a0a0a' : '#fafafa';
  const surface = dark ? '#111111' : '#ffffff';
  const foreground = dark ? '#ededed' : '#171717';
  const muted = dark ? '#a1a1a1' : '#595959';
  const border = dark ? '#333333' : '#e5e5e5';
  const hover = dark ? '#242424' : '#f2f2f2';
  const link = dark ? '#52a8ff' : '#0068d6';
  const page = {
    colors: [surface],
    shape: 'none',
    backgroundImage: 'none',
    fontColor: foreground,
  };

  return createUnifiedTheme({
    palette: {
      ...palettes[mode],
      primary: { main: foreground, contrastText: surface },
      secondary: { main: foreground, contrastText: surface },
      background: { default: background, paper: surface },
      text: { primary: foreground, secondary: muted },
      divider: border,
      border,
      textContrast: foreground,
      textSubtle: muted,
      textVerySubtle: muted,
      highlight: hover,
      link,
      linkHover: link,
      navigation: {
        background: surface,
        color: muted,
        selectedColor: foreground,
        indicator: foreground,
        navItem: { hoverBackground: hover },
      },
      tabbar: { indicator: foreground },
      code: { background: hover },
    },
    fontFamily,
    typography: {
      ...defaultTypography,
      fontFamily,
      h1: { fontSize: 36, fontWeight: 600, marginBottom: 8 },
      h2: { fontSize: 24, fontWeight: 600, marginBottom: 8 },
      h3: { fontSize: 24, fontWeight: 600, marginBottom: 6 },
      h4: { fontSize: 18, fontWeight: 600, marginBottom: 6 },
      h5: { fontSize: 16, fontWeight: 600, marginBottom: 4 },
      h6: { fontSize: 16, fontWeight: 600, marginBottom: 4 },
    },
    defaultPageTheme: 'home',
    // Unknown entity types fall back to home, keeping every header consistent.
    pageTheme: { home: page },
    components: {
      MuiCssBaseline: {
        styleOverrides: {
          body: { WebkitFontSmoothing: 'antialiased', colorScheme: mode },
          'code, pre, kbd, samp': { fontFamily: mono },
          'a:focus-visible, button:focus-visible, [tabindex]:focus-visible': {
            outline: `2px solid ${link}`,
            outlineOffset: 3,
          },
        },
      },
      MuiTypography: {
        styleOverrides: {
          body1: { fontSize: 14, lineHeight: 1.5 },
          body2: { fontSize: 13, lineHeight: 1.5 },
          subtitle1: { fontSize: 14, fontWeight: 500 },
          subtitle2: { fontSize: 13, fontWeight: 500 },
        },
      },
      MuiInputBase: { styleOverrides: { root: { fontSize: 14 } } },
      MuiMenuItem: {
        styleOverrides: {
          root: { fontSize: 14, minHeight: 36, '@media (pointer: coarse)': { minHeight: 44 } },
        },
      },
      BackstageSelect: {
        styleOverrides: {
          formControl: {
            margin: 0,
            '& label': { fontSize: 12, fontWeight: 500, color: muted },
          },
          label: { fontSize: 12, fontWeight: 500, color: muted },
        },
      },
      BackstageSelectInputBase: {
        styleOverrides: {
          root: { 'label + &': { marginTop: 20 } },
          input: {
            fontSize: 14,
            lineHeight: '20px',
            padding: '7px 28px 7px 10px',
            borderColor: border,
            borderRadius: 6,
            '@media (pointer: coarse)': { paddingTop: 11, paddingBottom: 11 },
          },
        },
      },
      BackstageAutocomplete: {
        styleOverrides: {
          root: { margin: 0 },
          label: { fontSize: 12, fontWeight: 500, color: muted },
        },
      },
      BackstageAutocompleteBase: {
        styleOverrides: {
          inputRoot: {
            marginTop: 20,
            minHeight: 36,
            borderRadius: 6,
            // CatalogAutocomplete's v4 withStyles rules have higher specificity
            // than unified-theme overrides; constrain only their vertical padding.
            paddingTop: '3px !important',
            paddingBottom: '3px !important',
            '& fieldset': { borderColor: border },
            '@media (pointer: coarse)': { minHeight: 44 },
          },
          input: {
            fontSize: '14px !important',
            lineHeight: '20px',
            paddingTop: '4px !important',
            paddingBottom: '4px !important',
          },
        },
      },
      CatalogReactUserListPicker: {
        styleOverrides: {
          root: { backgroundColor: 'transparent', border: 0, margin: '4px 0 8px' },
          title: {
            margin: '8px 0 4px',
            fontSize: 11,
            fontWeight: 500,
            letterSpacing: '0.04em',
            color: muted,
          },
          groupWrapper: { margin: 0, border: 0, backgroundColor: 'transparent', boxShadow: 'none' },
          menuItem: {
            minHeight: 34,
            padding: '2px 10px',
            border: 0,
            borderRadius: 6,
            marginBottom: 2,
            '&.Mui-selected': { backgroundColor: hover },
            '@media (pointer: coarse)': { minHeight: 44 },
          },
          listIcon: { minWidth: 26, color: muted, '& svg': { fontSize: 16 } },
        },
      },
      PluginCatalogTableToolbar: {
        styleOverrides: {
          root: {
            '@media (max-width: 600px)': {
              flexWrap: 'wrap', gap: 8, padding: '12px 16px',
              '& > *': { flex: '1 0 100%', width: '100%' },
              '& .MuiToolbar-root': { padding: 0, minHeight: 40 },
              '& .MuiFormControl-root': { width: '100%' },
            },
          },
          text: { fontSize: 16, fontWeight: 600 },
        },
      },
      BackstageTableToolbar: {
        styleOverrides: {
          root: {
            minHeight: 56, padding: '8px 16px',
            '@media (max-width: 600px)': {
              flexWrap: 'wrap', alignItems: 'center', gap: 8, padding: '12px 16px',
              '& > :first-child': { flex: '1 0 100%' },
            },
          },
          searchField: {
            paddingRight: 0,
            '& .MuiInput-root': {
              minHeight: 36, padding: '0 10px', borderRadius: 6,
              border: `1px solid ${border}`, backgroundColor: background,
              fontSize: 13,
              '&:before, &:after': { display: 'none' },
              '&:focus-within': { outline: `2px solid ${link}`, outlineOffset: 2 },
            },
            '& .MuiSvgIcon-root': { fontSize: 18, color: muted },
            '& .MuiIconButton-root': { padding: 6 },
            '@media (max-width: 600px)': { width: '100%', paddingLeft: 0 },
          },
          title: { fontSize: 16, fontWeight: 600 },
        },
      },
      BackstageTableHeader: {
        styleOverrides: {
          header: {
            fontSize: 12, fontWeight: 500, color: muted, textTransform: 'none',
            padding: '10px 16px', backgroundColor: background,
            borderTop: `1px solid ${border}`, borderBottom: `1px solid ${border}`,
          },
        },
      },
      MuiStepper: { styleOverrides: { root: { padding: '16px 24px' } } },
      MuiStepLabel: { styleOverrides: { label: { fontSize: 13 } } },
      MuiButton: {
        defaultProps: { disableElevation: true },
        styleOverrides: {
          root: { textTransform: 'none', fontSize: 13, fontWeight: 500, borderRadius: 6 },
          outlined: { borderColor: border },
        },
      },
      MuiPaper: {
        styleOverrides: {
          rounded: { borderRadius: 8 },
          elevation1: { boxShadow: 'none', border: `1px solid ${border}` },
          elevation2: { boxShadow: 'none', border: `1px solid ${border}` },
        },
      },
      MuiTableCell: {
        styleOverrides: {
          root: { borderBottom: `1px solid ${border}`, fontSize: 13, padding: '10px 16px' },
          head: { backgroundColor: background, color: muted, fontWeight: 500 },
        },
      },
      MuiTab: {
        styleOverrides: { root: { textTransform: 'none', fontSize: 13, minHeight: 44, fontWeight: 500 } },
      },
      MuiChip: {
        styleOverrides: { root: { borderRadius: 5, backgroundColor: hover } },
      },
      BackstageHeader: {
        styleOverrides: {
          header: {
            background: surface,
            boxShadow: 'none',
            borderBottom: `1px solid ${border}`,
            padding: '20px 24px',
          },
          title: { color: foreground, fontWeight: 600, letterSpacing: '-0.04em' },
          subtitle: { color: muted },
          type: { color: muted },
        },
      },
      BackstageHeaderTabs: {
        styleOverrides: {
          tabsWrapper: {
            backgroundColor: surface,
            borderBottom: `1px solid ${border}`,
          },
          defaultTab: { textTransform: 'none', color: muted },
          selected: { color: foreground },
        },
      },
    },
  });
}

const light = createGeistTheme('light');
const dark = createGeistTheme('dark');

// Keep Backstage's standard IDs so existing appearance preferences still work.
export const geistThemes = [
  {
    id: 'light',
    title: 'Geist Light',
    variant: 'light' as const,
    icon: <LightIcon />,
    Provider: ({ children }: PropsWithChildren<{}>) => (
      <UnifiedThemeProvider theme={light}>{children}</UnifiedThemeProvider>
    ),
  },
  {
    id: 'dark',
    title: 'Geist Dark',
    variant: 'dark' as const,
    icon: <DarkIcon />,
    Provider: ({ children }: PropsWithChildren<{}>) => (
      <UnifiedThemeProvider theme={dark}>{children}</UnifiedThemeProvider>
    ),
  },
];
