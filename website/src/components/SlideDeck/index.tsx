// Main exports for SlideDeck component library.
import './SlideContent.css';
import './SlideDrawer.css';
import './SlideImage.css';
import './SlideNotes.css';
import './Tooltip.css';

export { SlideDeck } from './SlideDeck';
export { Slide } from './Slide';
export { SlideTitle } from './SlideTitle';
export { SlideSubtitle } from './SlideSubtitle';
export { SlideContent } from './SlideContent';
export { SlideList } from './SlideList';
export { SlideCode } from './SlideCode';
export { SlideImage } from './SlideImage';
export { SlideSplit } from './SlideSplit';
export { SlideIndex } from './SlideIndex';
export { SlideDrawer } from './SlideDrawer';
export { SlideNotes } from './SlideNotes';
export { SlideNotesPanel } from './SlideNotesPanel';
export { Tooltip } from './Tooltip';

// Context exports for advanced usage.
export { SlideDeckProvider, useSlideDeck } from './SlideDeckContext';

// Type exports.
export type {
  SlideDeckProps,
  SlideProps,
  SlideTitleProps,
  SlideSubtitleProps,
  SlideContentProps,
  SlideListProps,
  SlideCodeProps,
  SlideImageProps,
  SlideSplitProps,
  SlideLayout,
  SlideDeckContextValue,
  SlideDeckMeta,
  SlideIndexProps,
  SlideNotesProps,
  SlideNotesPanelProps,
} from './types';
