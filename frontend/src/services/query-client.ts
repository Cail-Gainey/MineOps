import { MutationCache, QueryCache, QueryClient } from '@tanstack/vue-query'

import { reportClientError } from './error-reporter'

export const queryClient = new QueryClient({
  queryCache: new QueryCache({
    onError: (error) => reportClientError(error, 'query'),
  }),
  mutationCache: new MutationCache({
    onError: (error) => reportClientError(error, 'mutation'),
  }),
  defaultOptions: {
    queries: {
      retry: 1,
      staleTime: 30_000,
      refetchOnWindowFocus: false,
    },
    mutations: {
      retry: false,
    },
  },
})
