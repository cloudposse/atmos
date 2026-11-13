import {
  Link, Sidebar, SidebarDivider, SidebarGroup, SidebarItem,
  SidebarScrollWrapper, SidebarSpace, useSidebarOpenState,
} from '@backstage/core-components';
import type { NavContentComponentProps } from '@backstage/plugin-app-react';
import MenuIcon from '@material-ui/icons/Menu';
import SettingsIcon from '@material-ui/icons/Settings';
import { LogoFull, LogoIcon } from './PortalLogo';

function PortalBrand() {
  const { isOpen } = useSidebarOpenState();
  return (
    <div style={{ height: 72, display: 'flex', alignItems: 'center', paddingLeft: 24 }}>
      <Link to="/" underline="none" aria-label="Atmos service catalog">
        {isOpen ? <LogoFull /> : <LogoIcon />}
      </Link>
    </div>
  );
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
      </SidebarGroup>
    </Sidebar>
  );
}
