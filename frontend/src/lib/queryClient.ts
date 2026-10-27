import { QueryClient } from '@tanstack/react-query'

// QueryClient compartilhado entre App e AuthContext.
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5 * 60_000,
      gcTime: 10 * 60_000,
      refetchOnWindowFocus: false,
    },
  },
})
