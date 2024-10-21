import React, { useEffect, useState } from 'react';
import File from '@site/src/components/File';
import CodeBlock from '@theme/CodeBlock';

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

async function fetchFileContent(url: string): Promise<string | undefined> {
  try {
    const response = await fetch(url);
    if (!response.ok) {
      console.error(`Failed to fetch the file from URL: ${url} - ${response.statusText}`);
      return undefined;
    }
    return await response.text();
  } catch (error) {
    console.error('Error fetching the file:', error);
    return "Error fetching the file - " + error + "\n" + url;
  }
}

export default function RemoteFile({ source, language }) {
  const [fileContent, setFileContent] = useState<string>('Loading...');

  // Extract the filename from the URL
  const getFileName = (url: string) => {
    return url.substring(url.lastIndexOf('/') + 1);
  };

  const filePath = getFileName(source);
  const guessedLanguage = language || guessLanguageFromFilePath(filePath);

  useEffect(() => {
    const loadFileContent = async () => {
      const content = await fetchFileContent(source);
      setFileContent(content);
    };

    loadFileContent();
  }, [source]);

  return (
    <File title={filePath}>
        <CodeBlock className={`language-${guessedLanguage}`}>{fileContent}</CodeBlock>
    </File>
  );
}
