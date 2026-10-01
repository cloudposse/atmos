/**
 * SkillCount - Renders the live count of bundled Atmos agent skills inline in
 * prose, computed at build time from the same file-browser "skills" plugin
 * instance that powers /ai/skills. Never needs manual updates.
 */
import useGlobalData from '@docusaurus/useGlobalData';

interface GlobalDataFileBrowser {
  examples: { name: string }[];
}

export default function SkillCount(): JSX.Element {
  const globalData = useGlobalData();
  const skillsData = globalData['file-browser']?.['skills'] as GlobalDataFileBrowser | undefined;
  const count = skillsData?.examples.length;

  return <>{count ?? 'dozens of'}</>;
}
