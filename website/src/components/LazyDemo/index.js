import React, { useEffect, useRef, useState } from 'react';
import useBaseUrl from '@docusaurus/useBaseUrl';

export default function LazyDemo() {
  const [isVisible, setIsVisible] = useState(false);
  const imgRef = useRef(null);

  useEffect(() => {
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setIsVisible(true);
          // Once visible, stop observing
          observer.disconnect();
        }
      },
      {
        rootMargin: '100px', // Start loading 100px before it enters viewport
        threshold: 0.1,
      }
    );

    if (imgRef.current) {
      observer.observe(imgRef.current);
    }

    return () => {
      if (observer) {
        observer.disconnect();
      }
    };
  }, []);

  const demoUrl = useBaseUrl('/img/demo.gif');

  return (
    <div ref={imgRef} className="screenshot-container">
      {isVisible ? (
        <img
          src={demoUrl}
          alt="Atmos Demo"
          className="screenshot"
        />
      ) : (
        <div
          className="screenshot screenshot-placeholder"
          style={{
            background: 'rgba(255, 255, 255, 0.03)',
            border: '1px solid rgba(255, 255, 255, 0.1)',
            minHeight: '400px',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
          }}
        >
          <span style={{ color: '#666', fontSize: '0.9rem' }}>Loading demo...</span>
        </div>
      )}
    </div>
  );
}
