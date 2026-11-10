import React, { useRef, useState } from 'react';
import AtmosNewsletter from './index';
import styles from './dialog.module.css';

/** Native dialog provides focus containment, Escape dismissal, and focus restoration. */
export default function SubscribeButton(): JSX.Element {
  const dialog = useRef<HTMLDialogElement>(null);
  const [open, setOpen] = useState(false);
  return (
    <>
      <button
        type="button"
        className={styles.trigger}
        onClick={() => {
          setOpen(true);
          dialog.current?.showModal();
        }}
      >
        Subscribe
      </button>
      <dialog
        ref={dialog}
        className={styles.dialog}
        aria-label="Subscribe to Atmos updates"
        onClose={() => setOpen(false)}
        onClick={(event) => {
          if (event.target === event.currentTarget) dialog.current?.close();
        }}
      >
        <button
          type="button"
          className={styles.close}
          aria-label="Close subscription form"
          onClick={() => dialog.current?.close()}
        >
          <span aria-hidden="true">×</span>
        </button>
        {open && <AtmosNewsletter />}
      </dialog>
    </>
  );
}
