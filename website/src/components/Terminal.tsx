import React from 'react';

export default function Terminal({ title, className, children }) {
    return (
        <div className={className}>
            <div className="terminal">
                <div class="window-bar">
                    <div class="window-controls">
                        <div class="control-dot close-dot"></div>
                        <div class="control-dot minimize-dot"></div>
                        <div class="control-dot maximize-dot"></div>
                    </div>
                    <h1>{title}</h1>
                </div>
                <div className="viewport">{children}</div>
            </div>
        </div>
    );
};

