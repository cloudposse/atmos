import { useEffect, useRef } from 'react';
import { useSlideDeck } from './SlideDeckContext';

// Channel name for cross-window communication.
const CHANNEL_NAME = 'slide-deck-notes-sync';

// Message types for BroadcastChannel.
interface SyncMessage {
  type: 'slide-change' | 'notes-update' | 'close-popout' | 'navigate';
  slide?: number;
  notes?: string;
  direction?: 'next' | 'prev';
}

/**
 * SlideNotesPopout - Manages a separate browser window for speaker notes.
 *
 * Uses BroadcastChannel API for cross-window communication.
 * Shows current slide notes with navigation controls.
 */
export function SlideNotesPopout() {
  const {
    currentSlide,
    totalSlides,
    currentNotes,
    nextSlide,
    prevSlide,
    notesPreferences,
    setNotesPopout,
  } = useSlideDeck();

  const popoutWindowRef = useRef<Window | null>(null);
  const channelRef = useRef<BroadcastChannel | null>(null);

  // Initialize BroadcastChannel for cross-window sync.
  useEffect(() => {
    if (typeof BroadcastChannel !== 'undefined') {
      channelRef.current = new BroadcastChannel(CHANNEL_NAME);

      // Listen for messages from the popout window.
      channelRef.current.onmessage = (event: MessageEvent<SyncMessage>) => {
        const { type, direction } = event.data;
        if (type === 'navigate') {
          if (direction === 'next') {
            nextSlide();
          } else if (direction === 'prev') {
            prevSlide();
          }
        } else if (type === 'close-popout') {
          setNotesPopout(false);
        }
      };
    }

    return () => {
      channelRef.current?.close();
    };
  }, [nextSlide, prevSlide, setNotesPopout]);

  // Send slide updates to popout window.
  useEffect(() => {
    if (channelRef.current && notesPreferences.isPopout) {
      const message: SyncMessage = {
        type: 'slide-change',
        slide: currentSlide,
      };
      channelRef.current.postMessage(message);
    }
  }, [currentSlide, notesPreferences.isPopout]);

  // Open popout window when isPopout becomes true.
  // Only depends on isPopout - content updates happen in a separate effect.
  useEffect(() => {
    if (!notesPreferences.isPopout) {
      // Close the popout window if it exists.
      if (popoutWindowRef.current && !popoutWindowRef.current.closed) {
        popoutWindowRef.current.close();
      }
      popoutWindowRef.current = null;
      return;
    }

    // Open the popout window.
    const width = 400;
    const height = 500;
    const left = window.screenX + window.outerWidth - width - 50;
    const top = window.screenY + 50;

    const popout = window.open(
      '',
      'SlideNotesPopout',
      `width=${width},height=${height},left=${left},top=${top},menubar=no,toolbar=no,location=no,status=no`
    );

    if (!popout) {
      console.error('Failed to open popout window - popup may be blocked');
      setNotesPopout(false);
      return;
    }

    popoutWindowRef.current = popout;

    // Set up the popout window content.
    popout.document.title = 'Speaker Notes';

    // Write initial HTML structure (content will be updated by separate effect).
    popout.document.write(`
      <!DOCTYPE html>
      <html>
        <head>
          <title>Speaker Notes</title>
          <style>
            * {
              box-sizing: border-box;
              margin: 0;
              padding: 0;
            }
            body {
              font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
              background: #1a1a2e;
              color: rgba(255, 255, 255, 0.9);
              height: 100vh;
              display: flex;
              flex-direction: column;
            }
            .header {
              display: flex;
              justify-content: space-between;
              align-items: center;
              padding: 12px 16px;
              background: rgba(255, 255, 255, 0.05);
              border-bottom: 1px solid rgba(255, 255, 255, 0.1);
            }
            .title {
              font-size: 14px;
              font-weight: 600;
              display: flex;
              align-items: center;
              gap: 8px;
            }
            .slide-num {
              font-size: 13px;
              color: rgba(255, 255, 255, 0.6);
              font-variant-numeric: tabular-nums;
            }
            .nav-buttons {
              display: flex;
              gap: 8px;
            }
            .nav-btn {
              background: rgba(255, 255, 255, 0.1);
              border: none;
              color: rgba(255, 255, 255, 0.8);
              padding: 6px 12px;
              border-radius: 4px;
              cursor: pointer;
              font-size: 14px;
            }
            .nav-btn:hover:not(:disabled) {
              background: rgba(255, 255, 255, 0.2);
              color: #fff;
            }
            .nav-btn:disabled {
              opacity: 0.4;
              cursor: not-allowed;
            }
            .close-btn {
              background: transparent;
              border: none;
              color: rgba(255, 255, 255, 0.6);
              padding: 4px 8px;
              border-radius: 4px;
              cursor: pointer;
              font-size: 18px;
            }
            .close-btn:hover {
              background: rgba(255, 255, 255, 0.1);
              color: #fff;
            }
            .content {
              flex: 1;
              padding: 16px;
              overflow-y: auto;
              line-height: 1.7;
            }
            .content p {
              margin: 0 0 1em 0;
            }
            .content p:last-child {
              margin-bottom: 0;
            }
            .empty {
              display: flex;
              align-items: center;
              justify-content: center;
              height: 100%;
              color: rgba(255, 255, 255, 0.5);
              font-style: italic;
            }
          </style>
        </head>
        <body>
          <div class="header">
            <div class="title">
              <span>📝 Speaker Notes</span>
              <span class="slide-num" id="slide-num">Loading...</span>
            </div>
            <button class="close-btn" id="close-btn" title="Close and return to inline">×</button>
          </div>
          <div class="nav-buttons" style="padding: 8px 16px; background: rgba(255, 255, 255, 0.02);">
            <button class="nav-btn" id="prev-btn">← Previous</button>
            <button class="nav-btn" id="next-btn">Next →</button>
          </div>
          <div class="content" id="notes-content">
            <div class="empty">Loading notes...</div>
          </div>
          <script>
            const channel = new BroadcastChannel('${CHANNEL_NAME}');

            // Update notes content from main window.
            channel.onmessage = (event) => {
              const { type, slide } = event.data;
              if (type === 'slide-change') {
                // Content will be updated by React.
              }
            };

            // Navigation buttons.
            document.getElementById('prev-btn').addEventListener('click', () => {
              channel.postMessage({ type: 'navigate', direction: 'prev' });
            });

            document.getElementById('next-btn').addEventListener('click', () => {
              channel.postMessage({ type: 'navigate', direction: 'next' });
            });

            // Close button.
            document.getElementById('close-btn').addEventListener('click', () => {
              channel.postMessage({ type: 'close-popout' });
              window.close();
            });

            // Handle window close.
            window.addEventListener('beforeunload', () => {
              channel.postMessage({ type: 'close-popout' });
            });
          </script>
        </body>
      </html>
    `);
    popout.document.close();

    // Handle popout window being closed by user.
    const checkClosed = setInterval(() => {
      if (popout.closed) {
        clearInterval(checkClosed);
        setNotesPopout(false);
      }
    }, 500);

    return () => {
      clearInterval(checkClosed);
    };
  }, [notesPreferences.isPopout, setNotesPopout]);

  // Update the popout window content when notes change.
  useEffect(() => {
    if (!notesPreferences.isPopout || !popoutWindowRef.current || popoutWindowRef.current.closed) {
      return;
    }

    const popout = popoutWindowRef.current;
    const slideNumEl = popout.document.getElementById('slide-num');
    const notesContentEl = popout.document.getElementById('notes-content');
    const prevBtn = popout.document.getElementById('prev-btn') as HTMLButtonElement;
    const nextBtn = popout.document.getElementById('next-btn') as HTMLButtonElement;

    if (slideNumEl) {
      slideNumEl.textContent = `Slide ${currentSlide} / ${totalSlides}`;
    }

    if (prevBtn) {
      prevBtn.disabled = currentSlide === 1;
    }

    if (nextBtn) {
      nextBtn.disabled = currentSlide === totalSlides;
    }

    if (notesContentEl) {
      if (currentNotes) {
        // Convert React node to string if possible.
        // Use textContent for safety to prevent XSS.
        const notesText = typeof currentNotes === 'string'
          ? currentNotes
          : (currentNotes as React.ReactElement)?.props?.children || 'Notes available';
        // Clear existing content and create new div safely.
        notesContentEl.textContent = '';
        const div = popout.document.createElement('div');
        div.textContent = typeof notesText === 'string' ? notesText : String(notesText);
        notesContentEl.appendChild(div);
      } else {
        // Clear and create empty state safely.
        notesContentEl.textContent = '';
        const empty = popout.document.createElement('div');
        empty.className = 'empty';
        empty.textContent = 'No notes for this slide.';
        notesContentEl.appendChild(empty);
      }
    }
  }, [currentSlide, totalSlides, currentNotes, notesPreferences.isPopout]);

  // This component doesn't render anything in the main window.
  return null;
}

export default SlideNotesPopout;
