import React from 'react';

export interface DemoVideoProps {
  /** Demo video ID - matches scene name in demos/scenes.yaml. */
  id: string;
  /** Title for accessibility. */
  title: string;
  /** Whether to show caption below video. */
  showCaption?: boolean;
}

/**
 * DemoVideo component placeholder.
 *
 * This component will render demo videos once the demo infrastructure is added
 * (demos/scenes.yaml and video generation pipeline). For now, it returns null
 * to prevent build errors while keeping the component interface in place.
 *
 * Usage:
 *   <DemoVideo id="atmos-workflow" title="Atmos Workflow" showCaption />
 */
export default function DemoVideo(props: DemoVideoProps): JSX.Element | null {
  void props;
  // Placeholder: return null until demo infrastructure is added.
  // When implemented, this will:
  // 1. Look up the scene in demos/scenes.yaml by id
  // 2. Load the corresponding video/gif asset
  // 3. Render with lazy loading and optional caption
  return null;
}
