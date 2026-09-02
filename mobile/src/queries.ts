import { gql } from './api';
import type { Capacity, Contact, Request, Tier, User } from './types';

const CONTACT = `id tier createdAt user { id name }`;
const REQUEST = `id tier status createdAt from { id name } to { id name }`;

export type PeopleData = { contacts: Contact[]; capacity: Capacity };

export type RequestsData = {
  incomingRequests: Request[];
  outgoingRequests: Request[];
  contacts: Contact[];
  users: User[];
};

export const api = {
  users: () => gql<{ users: User[] }>(`{ users { id name } }`),

  people: (me: string) =>
    gql<PeopleData>(
      `{ contacts { ${CONTACT} } capacity { budgetUsed budgetCap tiers { tier used cap } } }`,
      {},
      me,
    ),

  requests: (me: string) =>
    gql<RequestsData>(
      `{ incomingRequests { ${REQUEST} } outgoingRequests { ${REQUEST} } contacts { ${CONTACT} } users { id name } }`,
      {},
      me,
    ),

  send: (me: string, toUserId: string, tier: Tier) =>
    gql<{ sendRequest: Request }>(
      `mutation($to: ID!, $tier: Tier!) { sendRequest(toUserId: $to, tier: $tier) { ${REQUEST} } }`,
      { to: toUserId, tier },
      me,
    ),

  accept: (me: string, requestId: string) =>
    gql<{ acceptRequest: Contact }>(
      `mutation($id: ID!) { acceptRequest(requestId: $id) { ${CONTACT} } }`,
      { id: requestId },
      me,
    ),

  decline: (me: string, requestId: string) =>
    gql<{ declineRequest: Request }>(
      `mutation($id: ID!) { declineRequest(requestId: $id) { ${REQUEST} } }`,
      { id: requestId },
      me,
    ),

  move: (me: string, contactId: string, tier: Tier) =>
    gql<{ moveContact: Contact }>(
      `mutation($id: ID!, $tier: Tier!) { moveContact(contactId: $id, tier: $tier) { ${CONTACT} } }`,
      { id: contactId, tier },
      me,
    ),

  remove: (me: string, contactId: string) =>
    gql<{ removeContact: boolean }>(
      `mutation($id: ID!) { removeContact(contactId: $id) }`,
      { id: contactId },
      me,
    ),
};
