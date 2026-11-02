import type { KeyboardEvent, MouseEvent, ReactNode } from 'react';

interface AddressLinkProps {
  address: string;
  intersection?: string;
  className?: string;
  children?: ReactNode;
}

export function AddressLink({ address, intersection, className, children }: AddressLinkProps) {
  const query = [address, intersection]
    .map((part) => part?.trim() || '')
    .filter(Boolean)
    .join(', ');

  if (!query) return <>{children || '—'}</>;

  const href = new URL('https://www.google.com/maps/search/');
  href.searchParams.set('api', '1');
  href.searchParams.set('query', query);

  const stopRowClick = (event: MouseEvent<HTMLAnchorElement>) => event.stopPropagation();
  const stopRowKeydown = (event: KeyboardEvent<HTMLAnchorElement>) => {
    if (event.key === 'Enter' || event.key === ' ') event.stopPropagation();
  };

  return (
    <a
      className={className ? `address-map-link ${className}` : 'address-map-link'}
      href={href.toString()}
      target="_blank"
      rel="noopener noreferrer"
      aria-label={`Open ${query} in Google Maps`}
      title="Open this location in Google Maps"
      onClick={stopRowClick}
      onKeyDown={stopRowKeydown}
    >
      {children || address}
    </a>
  );
}
