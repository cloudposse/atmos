// src/components/EmbedFile.js
import React, { useState, useEffect } from 'react';
import File from '@site/src/components/File'
import CodeBlock from '@theme/CodeBlock';

// Function to guess the type based on the filePath
const guessLanguageFromFilePath = (filePath) => {
    if (/\.ya?ml$/i.test(filePath)) {
        return 'yaml';
    }
    if (/\.json$/i.test(filePath)) {
        return 'json';
    }
    if (/\.tf$/i.test(filePath)) {
        return 'hcl';
    }
    // Add more patterns as needed
    return 'plain'; // Default to 'plain'
  };

const EmbedFile = ({ filePath, language }) => {
  const [content, setContent] = useState('');

  const guessedLanguage = language || guessLanguageFromFilePath(filePath);

  useEffect(() => {
    const loadFile = async () => {
      try {
        // Dynamically import the file content using raw-loader
        const fileContent = await import(`!!raw-loader!@site/${filePath}`);
        setContent(fileContent.default);
      } catch (error) {
        setContent(`Error loading file: ${error.message}`);
      }
    };

    loadFile();
  }, [filePath]);

  return (
    <File title={filePath}>
        <CodeBlock className={`language-${guessedLanguage}`}>{content}</CodeBlock>
    </File>
  );
};

export default EmbedFile;
