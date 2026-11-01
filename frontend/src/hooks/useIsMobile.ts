import { useEffect, useState } from 'react';

const MOBILE_QUERY = '(max-width: 760px)';

/**
 * True while the viewport matches the phone layout breakpoint.
 * Components use this to switch between desktop tables and mobile card lists,
 * which keeps the DOM simple instead of duplicating both in CSS.
 */
export function useIsMobile(): boolean {
  const [isMobile, setIsMobile] = useState(() =>
    typeof window !== 'undefined' && window.matchMedia(MOBILE_QUERY).matches
  );

  useEffect(() => {
    const queryList = window.matchMedia(MOBILE_QUERY);
    const onChange = (event: MediaQueryListEvent) => setIsMobile(event.matches);
    queryList.addEventListener('change', onChange);
    return () => queryList.removeEventListener('change', onChange);
  }, []);

  return isMobile;
}
