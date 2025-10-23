import React, { useEffect, useRef, useState } from 'react';

export default function ScrollFadeIn({ children, className = '' }) {
  const [isVisible, setIsVisible] = useState(false);
  const elementRef = useRef(null);

  useEffect(() => {
    const handleScroll = () => {
      // Fade in when user scrolls more than 50px
      if (window.scrollY > 50) {
        setIsVisible(true);
      } else {
        setIsVisible(false);
      }
    };

    // Check on mount
    handleScroll();

    // Listen for scroll events
    window.addEventListener('scroll', handleScroll, { passive: true });

    return () => {
      window.removeEventListener('scroll', handleScroll);
    };
  }, []);

  return (
    <div ref={elementRef} className={`${className} ${isVisible ? 'visible' : ''}`}>
      {children}
    </div>
  );
}
