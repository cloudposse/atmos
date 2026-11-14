import {
  Link, Sidebar, SidebarDivider, SidebarGroup, SidebarItem,
  SidebarScrollWrapper, SidebarSpace, useSidebarOpenState, useSidebarPinState,
} from '@backstage/core-components';
import { makeStyles } from '@material-ui/core/styles';
import type { NavContentComponentProps } from '@backstage/plugin-app-react';
import MenuIcon from '@material-ui/icons/Menu';
import SettingsIcon from '@material-ui/icons/Settings';
import ChevronLeftIcon from '@material-ui/icons/ChevronLeft';
import ChevronRightIcon from '@material-ui/icons/ChevronRight';
import { LogoIcon } from './PortalLogo';

const useStyles = makeStyles(theme => ({
  brand: {
    height: 64, width: 224, display: 'flex', alignItems: 'center', paddingLeft: 22,
    boxSizing: 'border-box',
    '& a': { display: 'flex', alignItems: 'center', gap: 12, color: theme.palette.text.primary },
  },
  wordmark: {
    fontSize: 18, fontWeight: 600, letterSpacing: '-0.04em', whiteSpace: 'nowrap',
    opacity: 0, transition: 'opacity 180ms ease-out',
    '&[data-open="true"]': { opacity: 1 },
    '@media (prefers-reduced-motion: reduce)': { transition: 'none' },
  },
}));

function PortalBrand() {
  const { isOpen } = useSidebarOpenState();
  const classes = useStyles();
  return (
    <div className={classes.brand}>
      <Link to="/" underline="none" aria-label="Atmos service catalog">
        <LogoIcon />
        <span className={classes.wordmark} data-open={isOpen} aria-hidden="true">
          atmos<span style={{ fontWeight: 400, opacity: 0.6 }}> / portal</span>
        </span>
      </Link>
    </div>
  );
}

function PortalPinButton() {
  const { isPinned, isMobile } = useSidebarPinState();
  const { setOpen } = useSidebarOpenState();
  if (isMobile) return null;
  return <SidebarItem
    icon={isPinned ? ChevronLeftIcon : ChevronRightIcon}
    text={isPinned ? 'Collapse sidebar' : 'Pin sidebar'}
    onClick={() => setOpen(!isPinned)}
  />;
}

export function PortalNav({ navItems }: NavContentComponentProps) {
  const nav = navItems.withComponent(item => (
    <SidebarItem icon={() => item.icon} to={item.href} text={item.title} />
  ));
  // Take settings before rest(), so it stays anchored at the bottom.
  const settings = nav.take('page:user-settings');
  return (
    <Sidebar>
      <PortalBrand />
      <SidebarDivider />
      <SidebarGroup label="Menu" icon={<MenuIcon />}>
        {nav.take('page:catalog')}
        {nav.take('page:scaffolder')}
        {nav.take('page:techdocs')}
        <SidebarDivider />
        <SidebarScrollWrapper>{nav.rest({ sortBy: 'title' })}</SidebarScrollWrapper>
      </SidebarGroup>
      <SidebarSpace />
      <SidebarDivider />
      <SidebarGroup label="Settings" icon={<SettingsIcon />} to="/settings">
        {settings}
        <PortalPinButton />
      </SidebarGroup>
    </Sidebar>
  );
}
