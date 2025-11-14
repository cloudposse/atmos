import React, { useEffect, useRef, useState } from 'react';

export default function ScrollFadeIn({ children, className = '' }) {
  const [isVisible, setIsVisible] = useState(false);
  const elementRef = useRef(null);

  useEffect(() => {
    const observer = new IntersectionObserver(
      ([entry]) => {
        setIsVisible(entry.isIntersecting);
      },
      {
        threshold: 0.1,
        rootMargin: '0px',
      }
    );

    if (elementRef.current) {
      observer.observe(elementRef.current);
    }

    return () => {
      if (observer) {
        observer.disconnect();
      }
    };
  }, []);

  return (
    <div ref={elementRef} className={`${className} ${isVisible ? 'visible' : ''}`}>
      {children}
    </div>
  );
}
