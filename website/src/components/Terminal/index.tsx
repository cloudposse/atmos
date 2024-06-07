import React from 'react';
import './index.css';

export default function Terminal({ title, className, children }) {
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
                <div className="viewport">{children}</div>
            </div>
        </div>
    );
};

