import { ReactNode } from 'react';

// Slide layout variants.
export type SlideLayout = 'title' | 'content' | 'split' | 'code' | 'quote';

// Props for the SlideDeck container component.
export interface SlideDeckProps {
  children: ReactNode;
  title?: string;
  showProgress?: boolean;
  showNavigation?: boolean;
  showFullscreen?: boolean;
  showDrawer?: boolean;
  startSlide?: number;
  className?: string;
}

// Props for individual Slide components.
export interface SlideProps {
  children: ReactNode;
  layout?: SlideLayout;
  background?: string;
  className?: string;
}

// Props for SlideTitle component.
export interface SlideTitleProps {
  children: ReactNode;
  className?: string;
}

// Props for SlideSubtitle component.
export interface SlideSubtitleProps {
  children: ReactNode;
  className?: string;
}

// Props for SlideContent component.
export interface SlideContentProps {
  children: ReactNode;
  className?: string;
}

// Props for SlideList component.
export interface SlideListProps {
  children: ReactNode;
  ordered?: boolean;
  className?: string;
}

// Props for SlideCode component.
export interface SlideCodeProps {
  children: string;
  language?: string;
  showLineNumbers?: boolean;
  className?: string;
}

// Props for SlideImage component.
export interface SlideImageProps {
  src: string;
  alt: string;
  className?: string;
  width?: number | string;
  height?: number | string;
  metallic?: boolean;
}

// Props for SlideSplit component.
export interface SlideSplitProps {
  children: ReactNode;
  ratio?: '1:1' | '1:2' | '2:1';
  className?: string;
}

// Context for slide navigation state.
export interface SlideDeckContextValue {
  currentSlide: number;
  totalSlides: number;
  goToSlide: (index: number) => void;
  nextSlide: () => void;
  prevSlide: () => void;
  isFullscreen: boolean;
  toggleFullscreen: () => void;
}

// Metadata for slide deck index page.
export interface SlideDeckMeta {
  title: string;
  description: string;
  thumbnail?: string;
  slideCount: number;
  tags?: string[];
  slug: string;
}

// Props for SlideIndex component.
export interface SlideIndexProps {
  decks: SlideDeckMeta[];
  className?: string;
}
