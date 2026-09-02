export type Tier = 'PINK' | 'BLUE' | 'GREEN';

/** Closest first, matching the server. */
export const TIERS: Tier[] = ['PINK', 'BLUE', 'GREEN'];

export const TIER_LABEL: Record<Tier, string> = { PINK: 'Pink', BLUE: 'Blue', GREEN: 'Green' };
export const TIER_COLOR: Record<Tier, string> = { PINK: '#c2185b', BLUE: '#1565c0', GREEN: '#2e7d32' };
export const TIER_TINT: Record<Tier, string> = { PINK: '#fce4ec', BLUE: '#e3f2fd', GREEN: '#e8f5e9' };

export type User = { id: string; name: string };

export type Contact = { id: string; user: User; tier: Tier; createdAt: string };

export type TierCapacity = { tier: Tier; used: number; cap: number };

/** budgetUsed may exceed budgetCap. Nothing here assumes otherwise. */
export type Capacity = { budgetUsed: number; budgetCap: number; tiers: TierCapacity[] };

export type RequestStatus = 'PENDING' | 'ACCEPTED' | 'DECLINED';

export type Request = {
  id: string;
  from: User;
  to: User;
  tier: Tier;
  status: RequestStatus;
  createdAt: string;
};
