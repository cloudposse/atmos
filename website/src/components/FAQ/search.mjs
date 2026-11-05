export function matchesQuery(text, query) {
  const haystack = text.toLocaleLowerCase();
  return query.trim().toLocaleLowerCase().split(/\s+/).every((word) => haystack.includes(word));
}

// Index rendered MDX once, including closed answers, without duplicating content.
export function createFAQController(root) {
  const items = Array.from(root.querySelectorAll('[data-faq-item]'));
  const sections = Array.from(root.querySelectorAll('[data-faq-section]'));
  const index = items.map((item) => ({
    item,
    text: `${item.querySelector('[data-faq-question]').textContent} ${item.querySelector('[data-faq-body]').textContent}`,
  }));
  let savedOpen = null;

  function search(query) {
    const searching = Boolean(query.trim());
    if (searching && savedOpen === null) {
      savedOpen = new Set(items.filter((item) => item.open));
    }
    let count = 0;
    for (const { item, text } of index) {
      const matches = matchesQuery(text, query);
      item.hidden = !matches;
      if (searching) item.open = matches;
      else if (savedOpen !== null) item.open = savedOpen.has(item);
      if (matches) count += 1;
    }
    if (!searching) savedOpen = null;
    for (const section of sections) {
      section.hidden = !Array.from(section.querySelectorAll('[data-faq-item]')).some((item) => !item.hidden);
    }
    return count;
  }

  function fragmentTarget(hash) {
    let id;
    try {
      id = decodeURIComponent(hash.replace(/^#/, ''));
    } catch {
      return null;
    }
    return [...items, ...sections].find((item) => item.id === id) || null;
  }

  function reveal(hash) {
    const target = fragmentTarget(hash);
    if (!target) return null;
    search('');
    if (items.includes(target)) target.open = true;
    return target;
  }

  return { search, reveal, total: items.length };
}
