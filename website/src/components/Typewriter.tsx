import React, { useEffect, useRef } from 'react';

export default function Typewriter({ className, children }) {
    const ref = useRef();

    useEffect(() => {
        const observer = new IntersectionObserver(entries => {
          entries.forEach(entry => {
            if (entry.isIntersecting) {
              entry.target.classList.add('animate');
            } else {
              entry.target.classList.remove('animate');
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
                <tt class="typing">{children}</tt>
            </div>
        </div>
    );
};
