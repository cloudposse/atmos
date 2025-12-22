import React, { useState, useRef, useEffect } from 'react';
import { Stream, StreamPlayerApi } from '@cloudflare/stream-react';
import { DemoVideo } from '../Video';
import { demoCategories, getDemoAssetUrls, getManifestData, getDemoById } from '../../data/demos';
import styles from './styles.module.css';

interface DemoCardProps {
  demo: {
    id: string;
    title: string;
    description: string;
    duration?: string;
    isPlaceholder?: boolean;
  };
  onClick: () => void;
}

function DemoCard({ demo, onClick }: DemoCardProps): JSX.Element {
  const { png, streamUid } = getDemoAssetUrls(demo.id);
  const manifestData = getManifestData(demo.id);
  const videoUid = streamUid || manifestData?.formats?.mp4?.uid;

  const [isHovered, setIsHovered] = useState(false);
  const [hasBeenHovered, setHasBeenHovered] = useState(false);
  const [isPlaying, setIsPlaying] = useState(false);
  const streamRef = useRef<StreamPlayerApi | undefined>(undefined);

  // Once hovered, keep video mounted to preserve buffer quality.
  useEffect(() => {
    if (isHovered && videoUid) {
      setHasBeenHovered(true);
      // Start playing when hovered.
      if (streamRef.current) {
        streamRef.current.play();
      }
    } else if (!isHovered && streamRef.current) {
      // Pause and reset when not hovered.
      streamRef.current.pause();
      streamRef.current.currentTime = 0;
      setIsPlaying(false);
    }
  }, [isHovered, videoUid]);

  const handleMouseEnter = () => {
    setIsHovered(true);
  };

  const handleMouseLeave = () => {
    setIsHovered(false);
  };

  const handlePlay = () => {
    setIsPlaying(true);
  };

  const handleCanPlay = () => {
    // When video is ready and we're hovered, start playing.
    if (isHovered && streamRef.current) {
      streamRef.current.play();
    }
  };

  // Placeholder cards are not clickable
  const handleClick = demo.isPlaceholder ? undefined : onClick;

  return (
    <div
      className={`${styles.card} ${demo.isPlaceholder ? styles.placeholder : ''}`}
      onClick={handleClick}
      onMouseEnter={handleMouseEnter}
      onMouseLeave={handleMouseLeave}
    >
      <div className={`${styles.thumbnail} ${!png ? styles.noThumbnail : ''}`}>
        {png && <img src={png} alt={demo.title} loading="lazy" />}
        {hasBeenHovered && videoUid && (
          <div className={`${styles.videoPreview} ${isHovered && isPlaying ? styles.videoVisible : ''}`}>
            <Stream
              src={videoUid}
              controls={false}
              autoplay={false}
              muted={true}
              loop={true}
              preload="auto"
              streamRef={streamRef}
              onPlay={handlePlay}
              onCanPlay={handleCanPlay}
              responsive
            />
          </div>
        )}
        {demo.isPlaceholder ? (
          <span className={styles.comingSoon}>Coming Soon</span>
        ) : (
          <div className={styles.playButton}>
            <svg viewBox="0 0 24 24" fill="currentColor">
              <path d="M8 5v14l11-7z" />
            </svg>
          </div>
        )}
        {demo.duration && (
          <span className={styles.duration}>{demo.duration}</span>
        )}
      </div>
      <div className={styles.cardContent}>
        <h4 className={styles.cardTitle}>{demo.title}</h4>
        <p className={styles.cardDescription}>{demo.description}</p>
      </div>
    </div>
  );
}

interface DemoModalProps {
  demo: {
    id: string;
    title: string;
    description: string;
  } | null;
  onClose: () => void;
  onShare: (id: string) => void;
  copied: boolean;
}

function DemoModal({ demo, onClose, onShare, copied }: DemoModalProps): JSX.Element | null {
  if (!demo) return null;

  return (
    <div className={styles.modalOverlay} onClick={onClose}>
      <div className={styles.modal} onClick={(e) => e.stopPropagation()}>
        <div className={styles.modalButtons}>
          <button
            className={`${styles.shareButton} ${copied ? styles.copied : ''}`}
            onClick={() => onShare(demo.id)}
            title={copied ? "Copied!" : "Copy link"}
          >
            <svg viewBox="0 0 24 24" fill="currentColor">
              {copied ? (
                <path d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z" />
              ) : (
                <path d="M18 16.08c-.76 0-1.44.3-1.96.77L8.91 12.7c.05-.23.09-.46.09-.7s-.04-.47-.09-.7l7.05-4.11c.54.5 1.25.81 2.04.81 1.66 0 3-1.34 3-3s-1.34-3-3-3-3 1.34-3 3c0 .24.04.47.09.7L8.04 9.81C7.5 9.31 6.79 9 6 9c-1.66 0-3 1.34-3 3s1.34 3 3 3c.79 0 1.5-.31 2.04-.81l7.12 4.16c-.05.21-.08.43-.08.65 0 1.61 1.31 2.92 2.92 2.92s2.92-1.31 2.92-2.92-1.31-2.92-2.92-2.92z" />
              )}
            </svg>
          </button>
          <button className={styles.closeButton} onClick={onClose}>
            <svg viewBox="0 0 24 24" fill="currentColor">
              <path d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z" />
            </svg>
          </button>
        </div>
        <DemoVideo
          id={demo.id}
          title={demo.title}
          description={demo.description}
        />
      </div>
    </div>
  );
}

export default function DemoGallery(): JSX.Element {
  const [selectedDemo, setSelectedDemo] = useState<{
    id: string;
    title: string;
    description: string;
  } | null>(null);
  const [activeCategory, setActiveCategory] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  // Read URL hash on mount to open deep-linked video.
  useEffect(() => {
    const hash = window.location.hash;
    if (hash) {
      const videoId = hash.slice(1); // Remove leading #
      const demo = getDemoById(videoId);
      if (demo) setSelectedDemo(demo);
    }
  }, []);

  // Update URL hash when modal opens/closes.
  useEffect(() => {
    if (selectedDemo) {
      window.history.replaceState({}, '', `/demos#${selectedDemo.id}`);
    } else {
      window.history.replaceState({}, '', '/demos');
    }
  }, [selectedDemo]);

  const handleShare = async (demoId: string) => {
    const url = `${window.location.origin}/demos#${demoId}`;
    await navigator.clipboard.writeText(url);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const filteredCategories = activeCategory
    ? demoCategories.filter((cat) => cat.id === activeCategory)
    : demoCategories;

  return (
    <div className={styles.gallery}>
      {/* Category Filter */}
      <div className={styles.categoryFilter}>
        <button
          className={`${styles.filterButton} ${!activeCategory ? styles.active : ''}`}
          onClick={() => setActiveCategory(null)}
        >
          All
        </button>
        {demoCategories.map((category) => (
          <button
            key={category.id}
            className={`${styles.filterButton} ${activeCategory === category.id ? styles.active : ''}`}
            onClick={() => setActiveCategory(category.id)}
          >
            {category.title}
          </button>
        ))}
      </div>

      {/* Demo Categories */}
      {filteredCategories.map((category) => (
        <section key={category.id} className={styles.category}>
          <div className={styles.categoryHeader}>
            <h2 className={styles.categoryTitle}>{category.title}</h2>
            <p className={styles.categoryDescription}>{category.description}</p>
          </div>
          <div className={styles.grid}>
            {category.demos.map((demo) => (
              <DemoCard
                key={demo.id}
                demo={demo}
                onClick={() => setSelectedDemo(demo)}
              />
            ))}
          </div>
        </section>
      ))}

      {/* Modal */}
      <DemoModal
        demo={selectedDemo}
        onClose={() => setSelectedDemo(null)}
        onShare={handleShare}
        copied={copied}
      />
    </div>
  );
}
