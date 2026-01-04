import { useEffect } from 'react';
import { useSlideDeck } from './SlideDeckContext';
import type { SlideNotesProps } from './types';

/**
 * SlideNotes - A component for adding speaker notes to slides.
 *
 * This component does not render any visible content. Instead, it registers
 * its children (the notes content) with the SlideDeck context so that the
 * notes can be displayed in the SlideNotesPanel when the user presses 'N'.
 *
 * Usage in MDX:
 * ```mdx
 * <Slide>
 *   <SlideTitle>My Slide</SlideTitle>
 *   <SlideContent>...</SlideContent>
 *   <SlideNotes>
 *     Speaker notes go here. Can be multiple paragraphs.
 *     The presenter sees these when pressing 'N'.
 *   </SlideNotes>
 * </Slide>
 * ```
 */
export function SlideNotes({ children }: SlideNotesProps) {
  const { setCurrentNotes } = useSlideDeck();

  useEffect(() => {
    // Register notes with the context when this component mounts.
    setCurrentNotes(children);

    // Clear notes when unmounting (slide changes).
    return () => {
      setCurrentNotes(null);
    };
  }, [children, setCurrentNotes]);

  // This component renders nothing - notes are displayed in SlideNotesPanel.
  return null;
}

export default SlideNotes;
