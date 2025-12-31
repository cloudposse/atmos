const fs = require('fs');
const path = require('path');

module.exports = function slideNotesExtractorPlugin(context, options) {
  return {
    name: 'slide-notes-extractor',

    async postBuild({ outDir }) {
      const slidesDir = path.join(context.siteDir, 'docs/slides');

      // Check if slides directory exists.
      if (!fs.existsSync(slidesDir)) {
        console.log('[slide-notes-extractor] No docs/slides directory found, skipping');
        return;
      }

      // Find all MDX files in docs/slides/.
      const mdxFiles = fs.readdirSync(slidesDir)
        .filter(f => f.endsWith('.mdx'));

      if (mdxFiles.length === 0) {
        console.log('[slide-notes-extractor] No MDX files found in docs/slides/');
        return;
      }

      let totalNotes = 0;

      for (const file of mdxFiles) {
        const content = fs.readFileSync(
          path.join(slidesDir, file),
          'utf-8'
        );

        // Extract notes from each slide.
        const notes = extractSlideNotes(content);

        // Create output directory.
        const deckName = path.basename(file, '.mdx');
        const outputDir = path.join(outDir, 'slides', deckName);
        fs.mkdirSync(outputDir, { recursive: true });

        // Write individual .txt files (only for slides with notes).
        let notesCount = 0;
        notes.forEach((noteText, index) => {
          if (noteText.trim()) {
            fs.writeFileSync(
              path.join(outputDir, `slide${index + 1}.txt`),
              noteText.trim()
            );
            notesCount++;
          }
        });

        totalNotes += notesCount;
        console.log(`[slide-notes-extractor] ${deckName}: ${notesCount} notes from ${notes.length} slides`);
      }

      console.log(`[slide-notes-extractor] Total: ${totalNotes} speaker notes extracted`);
    }
  };
};

/**
 * Extract speaker notes from MDX content.
 * Returns an array of note strings, one per slide (empty string if no notes).
 */
function extractSlideNotes(mdxContent) {
  const notes = [];

  // Regex to match <Slide>...</Slide> blocks (handles attributes and multiline).
  const slideRegex = /<Slide[^>]*>([\s\S]*?)<\/Slide>/g;

  let match;
  while ((match = slideRegex.exec(mdxContent)) !== null) {
    const slideContent = match[1];

    // Extract <SlideNotes>...</SlideNotes> content.
    const notesMatch = slideContent.match(/<SlideNotes>([\s\S]*?)<\/SlideNotes>/);

    if (notesMatch) {
      // Strip JSX/HTML tags, normalize whitespace for plain text output.
      let text = notesMatch[1];

      // Remove JSX comments first.
      text = text.replace(/\{\/\*[\s\S]*?\*\/\}/g, '');

      // Use a simple, secure approach: extract only text content.
      // Replace all HTML/JSX tags with spaces (to preserve word boundaries),
      // then clean up. This avoids multi-character sanitization issues.
      //
      // Strategy: Rather than trying to strip tags iteratively (which can
      // leave fragments), we extract text between tags and join with spaces.
      const textParts = [];
      let lastIndex = 0;
      const tagRegex = /<[^>]*>/g;
      let tagMatch;

      while ((tagMatch = tagRegex.exec(text)) !== null) {
        // Extract text before this tag.
        if (tagMatch.index > lastIndex) {
          textParts.push(text.slice(lastIndex, tagMatch.index));
        }
        lastIndex = tagMatch.index + tagMatch[0].length;
      }
      // Extract remaining text after last tag.
      if (lastIndex < text.length) {
        textParts.push(text.slice(lastIndex));
      }

      // Join parts and clean up any stray angle brackets (from malformed input).
      text = textParts.join(' ').replace(/[<>]/g, '');

      // Normalize whitespace.
      text = text.replace(/\s+/g, ' ').trim();

      notes.push(text);
    } else {
      notes.push(''); // Empty string for slides without notes.
    }
  }

  return notes;
}
