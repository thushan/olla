// Filter state for the Endpoints panel, held outside the component so the
// jump-to-endpoint helper can clear it: a filter that hides the navigation
// target (a Back press, a shared link, a Models pill) would otherwise leave
// the jump with nothing to reveal.
import type { StatusFilter } from '../herd';

class EndpointFilter {
  query = $state('');
  status: StatusFilter = $state('all');

  clear(): void {
    this.query = '';
    this.status = 'all';
  }
}

export const endpointFilter = new EndpointFilter();
