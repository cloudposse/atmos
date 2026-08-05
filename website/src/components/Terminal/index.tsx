import React, { createContext, useContext } from 'react';
import Typewriter from '@site/src/components/Typewriter';
import './index.css';

const TerminalContext = createContext(false);

export function useIsInsideTerminal(): boolean {
    return useContext(TerminalContext);
}

export default function Terminal({ title, command, className, children }) {
    return (
        <div className={className}>
            <TerminalContext.Provider value={true}>
                <div className="terminal">
                    <div className="window-bar">
                        <div className="window-controls" aria-hidden="true">
                            <div className="control-dot close-dot"></div>
                            <div className="control-dot minimize-dot"></div>
                            <div className="control-dot maximize-dot"></div>
                        </div>
                        <div className="window-title">{title}</div>
                    </div>
                    <div className="viewport">
                        {command && <div className="command"><span className="prompt">&gt;</span><Typewriter>{command}</Typewriter></div>}
                        <div>{children}</div>
                    </div>
                </div>
            </TerminalContext.Provider>
        </div>
    );
};
