/**
 * SourceLink - "View on GitHub" button component.
 */
import React from 'react';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faGithub } from '@fortawesome/free-brands-svg-icons';
import { faExternalLinkAlt } from '@fortawesome/free-solid-svg-icons';
import styles from './styles.module.css';

interface SourceLinkProps {
  url: string;
  label?: string;
}

export default function SourceLink({ url, label = 'View on GitHub' }: SourceLinkProps): JSX.Element {
  return (
    <a
      href={url}
      target="_blank"
      rel="noopener noreferrer"
      className={styles.githubButton}
    >
      <FontAwesomeIcon icon={faGithub} className={styles.githubButtonIcon} />
      <span>{label}</span>
      <FontAwesomeIcon icon={faExternalLinkAlt} className={styles.externalIcon} />
    </a>
  );
}
