import React from 'react';

export default function CardGroup({ title, className, children }) {
    return (
        <div className={className}>
            <h2>{title}</h2>
            <div className="card-group">{children}</div>
        </div>
    );
};

