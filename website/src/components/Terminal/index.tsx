import React from 'react';
import Typewriter from '@site/src/components/Typewriter';
import './index.css';

export default function Terminal({ title, command, className, children }) {
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
                <div className="viewport">{command && <div className="command"><span className="prompt">&gt;</span><Typewriter>{command}</Typewriter></div>}<div>{children}</div></div>
            </div>
        </div>
    );
};

