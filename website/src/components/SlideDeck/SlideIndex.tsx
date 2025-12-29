import React from 'react';
import Link from '@docusaurus/Link';
import type { SlideIndexProps, SlideDeckMeta } from './types';
import './SlideIndex.css';

function SlideCard({ deck }: { deck: SlideDeckMeta }) {
  return (
    <Link to={deck.slug} className="slide-index__card">
      {deck.thumbnail && (
        <div className="slide-index__thumbnail">
          <img src={deck.thumbnail} alt={`${deck.title} thumbnail`} />
        </div>
      )}
      <div className="slide-index__card-content">
        <h3 className="slide-index__card-title">{deck.title}</h3>
        <p className="slide-index__card-description">{deck.description}</p>
        <div className="slide-index__card-meta">
          <span className="slide-index__slide-count">{deck.slideCount} slides</span>
          {deck.tags && deck.tags.length > 0 && (
            <div className="slide-index__tags">
              {deck.tags.map((tag) => (
                <span key={tag} className="slide-index__tag">{tag}</span>
              ))}
            </div>
          )}
        </div>
      </div>
    </Link>
  );
}

export function SlideIndex({ decks, className = '' }: SlideIndexProps) {
  if (decks.length === 0) {
    return (
      <div className={`slide-index slide-index--empty ${className}`}>
        <p>No slide decks available.</p>
      </div>
    );
  }

  return (
    <div className={`slide-index ${className}`}>
      <div className="slide-index__grid">
        {decks.map((deck) => (
          <SlideCard key={deck.slug} deck={deck} />
        ))}
      </div>
    </div>
  );
}

export default SlideIndex;
