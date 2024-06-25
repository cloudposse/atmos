import React, { useEffect, useRef } from 'react';
import './index.css';

export default function Typewriter({ className, children }) {
    const ref = useRef();

    useEffect(() => {
        let timer;
        const observer = new IntersectionObserver(entries => {
          entries.forEach(entry => {
            if (entry.isIntersecting) {
              entry.target.classList.add('animate');
              timer = setTimeout(() => {
                  entry.target.classList.add('hiddenCursor');
              }, 3000); // Adjust the timeout duration as needed
            } else {
              entry.target.classList.remove('animate', 'hiddenCursor');
              clearTimeout(timer);
            }
          });
        });

        if (ref.current) {
          observer.observe(ref.current);
        }

        return () => {
          if (ref.current) {
            observer.unobserve(ref.current);
          }
        };
      }, []);

    return (
        <div ref={ref} className={className}>
            <div className="typewriter">
                <tt className="typing">{children}</tt>
            </div>
        </div>
    );
};
