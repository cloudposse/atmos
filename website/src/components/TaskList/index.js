import React, { useEffect, useRef } from 'react';
import clsx from 'clsx';
import styles from './index.module.css';

const hashString = (str) => {
  let hash = 0;
  for (let i = 0; i < str.length; i++) {
    const char = str.charCodeAt(i);
    hash = (hash << 5) - hash + char;
    hash |= 0; // Convert to 32-bit integer
  }
  return hash;
};

const TaskList = ({ children }) => {
  const listRef = useRef(null);

  useEffect(() => {
    const listContainer = listRef.current;
    if (listContainer) {
      const lists = listContainer.querySelectorAll('ul, ol');
      lists.forEach((list) => {
        list.classList.add('contains-task-list');

        const listItems = list.querySelectorAll('li');
        listItems.forEach((li) => {
          li.classList.add('task-list-item');

          const firstChild = li.firstChild;
          let checkbox;
          if (!firstChild || !(firstChild.nodeType === Node.ELEMENT_NODE && firstChild.tagName === 'INPUT' && firstChild.type === 'checkbox')) {
            // If the first element is not a checkbox, inject one.
            checkbox = document.createElement('input');
            checkbox.type = 'checkbox';
            checkbox.classList.add('task-list-checkbox');
            li.insertBefore(checkbox, firstChild);
          } else {
            checkbox = firstChild;
            firstChild.classList.add('task-list-checkbox');
          }

          // Enable the checkbox if it is disabled.
          if (checkbox.disabled) {
            checkbox.disabled = false;
          }

          // Generate a checksum for the list item content.
          const itemContent = li.textContent || '';
          const itemChecksum = hashString(itemContent);

          // Set the checkbox state from localStorage.
          const storedState = localStorage.getItem(`task-checkbox-${itemChecksum}`);
          if (storedState) {
            checkbox.checked = storedState === 'true';
          }

          // Add event listener to save state to localStorage.
          checkbox.addEventListener('change', () => {
            localStorage.setItem(`task-checkbox-${itemChecksum}`, checkbox.checked);
          });
        });
      });
    }
  }, []);

  return (
    <div ref={listRef} className={clsx(styles.taskList)}>
      {children}
    </div>
  );
};

export default TaskList;
