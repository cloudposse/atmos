import React, { useState, useEffect } from 'react';
import Typewriter from '@site/src/components/Typewriter';

export default function Screengrab({ title, command, className, slug, children }) {
    const [html, setHtml] = useState(null);
    const [missing, setMissing] = useState(false);

    useEffect(() => {
        let cancelled = false;

        setHtml(null);
        setMissing(false);

        import(`@site/src/components/Screengrabs/${slug}.html`)
            .then(module => {
                if (!cancelled) {
                    setHtml(module.default);
                }
            })
            .catch(error => {
                console.warn(`Missing screengrab artifact: ${slug}.html`, error);

                if (!cancelled) {
                    setMissing(true);
                }
            });

        return () => {
            cancelled = true;
        };
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
                    <div className="window-title">{title}</div>
                </div>
                <div className="viewport">
                    {command && <Typewriter>{command}</Typewriter>}
                    {children}
                    {html && <pre className="screengrab" dangerouslySetInnerHTML={{ __html: html }}></pre>}
                    {missing && <pre className="screengrab screengrab--missing">Missing screengrab: {slug}.html</pre>}
                </div>
            </div>
        </div>
    );
};
