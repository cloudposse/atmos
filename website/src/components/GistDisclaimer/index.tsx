import React from 'react';
import { RiLightbulbFlashLine } from 'react-icons/ri';
import './index.css';

interface GistDisclaimerProps {
  text?: string;
}

const GistDisclaimer: React.FC<GistDisclaimerProps> = ({
  text = 'Gists are examples that demonstrate a concept, but are not actively maintained and may not work in your environment or current versions of Atmos without adaptations.',
}) => {
  return (
    <div className="gist-disclaimer" role="note" aria-label="Gist disclaimer">
      <RiLightbulbFlashLine className="gist-disclaimer-icon" />
      <span className="gist-disclaimer-text">{text}</span>
    </div>
  );
};

export default GistDisclaimer;
