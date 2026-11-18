import { useTheme } from '@material-ui/core/styles';

// A simple Atmos stack mark; no external image or brand assets.
export const LogoIcon = () => (
  <svg width="28" height="28" viewBox="0 0 28 28" fill="none" aria-hidden="true" style={{ flexShrink: 0 }}>
    <path d="M3 9L14 3L25 9L14 15L3 9Z" stroke="currentColor" strokeWidth="1.5" />
    <path d="M3 14L14 20L25 14M3 19L14 25L25 19" stroke="currentColor" strokeWidth="1.5" />
  </svg>
);

export const LogoFull = () => {
  const theme = useTheme();
  return (
    <span style={{ display: 'flex', alignItems: 'center', gap: 10, color: theme.palette.text.primary }}>
      <LogoIcon />
      <span style={{ fontSize: 22, fontWeight: 600, letterSpacing: '-0.04em', whiteSpace: 'nowrap' }}>
        atmos<span style={{ fontWeight: 400, opacity: 0.6 }}> / portal</span>
      </span>
    </span>
  );
};
