import React, { useState, useEffect } from 'react';
import Typewriter from '@site/src/components/Typewriter';

export default function Screengrab({ title, command, className, slug, children }) {
    const [html, setHtml] = useState(null);

    useEffect(() => {
        import(`@site/src/components/screengrabs/${slug}.html`)
            .then(module => {
                setHtml(module.default);
            });
    }, [slug]);

    return (
        <div className={className}>
            <div className="terminal">
                <div className="window-bar">
                    <div className="window-controls">
                        <div className="control-dot close-dot"></div>
                        <div className="control-dot minimize-dot"></div>
                        <div className="control-dot maximize-dot"></div>
                    </div>
                    <h1>{title}</h1>
                </div>
                <div className="viewport">
                    {command && <Typewriter>{command}</Typewriter>}
                    {children}
                    {html && <pre className="screengrab" dangerouslySetInnerHTML={{ __html: html }}></pre>}
                </div>
            </div>
        </div>
    );
};
