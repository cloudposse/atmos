import React, { useState, useEffect } from 'react';
import { MDXProvider } from '@mdx-js/react';
import './index.css';

const Definition = ({ term, children }) => {
    const [Content, setContent] = useState(null);
    const [error, setError] = useState(false);

    useEffect(() => {
        const loadContent = async () => {
            try {
                console.log(`Trying to load ${term}.mdx`);
                const mdxContent = await import(`@site/docs/glossary/${term}.mdx`);
                setContent(() => mdxContent.default);
                console.log(`${term}.mdx loaded successfully`);
            } catch (err1) {
                console.log(`Failed to load ${term}.mdx, trying ${term}.md`);
                try {
                    const mdContent = await import(`@site/docs/glossary/${term}.md`);
                    setContent(() => mdContent.default);
                    console.log(`${term}.md loaded successfully`);
                } catch (err2) {
                    console.error(`Failed to load both ${term}.mdx and ${term}.md`);
                    setError(true);
                }
            }
        };

        loadContent();
    }, [term]);

    return (
        <details className="definition">
            <summary>{children || `What is a ${term}?`}</summary>
            <div>
                {error ? (
                    <p>Definition not found.</p>
                ) : Content ? (
                    <MDXProvider>
                        <Content />
                    </MDXProvider>
                ) : (
                    <p>Loading...</p>
                )}
            </div>
        </details>
    );
};

export default Definition;
