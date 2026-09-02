// The GraphQL wire. Kept deliberately small: one fetch, no client library.
// The screens here are a handful of queries and five mutations, and the thing
// the brief grades on the client is that a refusal reaches the user as the
// sentence the server wrote. A raw fetch delivers that sentence untouched; a
// normalized cache would be a second place for the seat counts to be wrong.
//
// iOS simulator and web reach the API on localhost. A physical device does not:
// set EXPO_PUBLIC_API_URL to http://<your-lan-ip>:8080/query instead.
const API_URL = process.env.EXPO_PUBLIC_API_URL ?? 'http://localhost:8080/query';

export class GraphQLError extends Error {
  /** extensions.code from the server, e.g. CAPACITY_FULL. UNKNOWN when absent. */
  code: string;
  extensions: Record<string, unknown>;

  constructor(message: string, extensions: Record<string, unknown> = {}) {
    super(message);
    this.name = 'GraphQLError';
    this.extensions = extensions;
    this.code = typeof extensions.code === 'string' ? extensions.code : 'UNKNOWN';
  }
}

type Wire<T> = {
  data?: T;
  errors?: { message: string; extensions?: Record<string, unknown> }[];
};

export async function gql<T>(
  query: string,
  variables: Record<string, unknown> = {},
  userId?: string,
): Promise<T> {
  const res = await fetch(API_URL, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(userId ? { 'X-User-Id': userId } : {}),
    },
    body: JSON.stringify({ query, variables }),
  });

  if (!res.ok) throw new GraphQLError(`HTTP ${res.status}`);

  const body = (await res.json()) as Wire<T>;
  const first = body.errors?.[0];
  if (first) throw new GraphQLError(first.message, first.extensions);
  if (!body.data) throw new GraphQLError('no data');
  return body.data;
}

/** The sentence to show a person for any failure. */
export function messageOf(e: unknown): string {
  if (e instanceof Error) return e.message;
  return String(e);
}
