// ============================================================
// useTypewriter — 打字机效果 Hook
// ============================================================
import { useEffect, useState } from 'react';

export default function useTypewriter(texts, typeSpeed = 55, deleteSpeed = 28, pause = 1600) {
  const [display, setDisplay] = useState('');
  const [textIndex, setTextIndex] = useState(0);

  useEffect(() => {
    const current = texts[textIndex % texts.length];
    let timeout;
    let i = 0;
    let deleting = false;

    const tick = () => {
      if (!deleting) {
        i++;
        setDisplay(current.slice(0, i));
        if (i >= current.length) {
          deleting = true;
          timeout = setTimeout(tick, pause);
          return;
        }
        timeout = setTimeout(tick, typeSpeed);
      } else {
        i--;
        setDisplay(current.slice(0, i));
        if (i <= 0) {
          setTextIndex((idx) => (idx + 1) % texts.length);
          return;
        }
        timeout = setTimeout(tick, deleteSpeed);
      }
    };

    timeout = setTimeout(tick, 400);
    return () => clearTimeout(timeout);
  }, [textIndex, texts, typeSpeed, deleteSpeed, pause]);

  return display;
}
