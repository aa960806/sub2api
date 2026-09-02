import { describe, expect, it } from "vitest";

import {
  normalizeSubNexusInviteRewardSettings,
  SUBNEXUS_INVITE_REWARD_DEFAULTS,
} from "@/api/admin/settings";

describe("admin settings invite signup rewards", () => {
  it("fails closed when the backend omits the new settings", () => {
    expect(normalizeSubNexusInviteRewardSettings()).toEqual(
      SUBNEXUS_INVITE_REWARD_DEFAULTS,
    );
  });

  it("normalizes valid values while keeping the independent switch", () => {
    expect(
      normalizeSubNexusInviteRewardSettings({
        subnexus_invite_rewards_enabled: true,
        subnexus_invite_reward_inviter_amount: 1.25,
        subnexus_invite_reward_invitee_amount: 0.5,
        subnexus_invite_reward_ip_limit_enabled: true,
        subnexus_invite_reward_ip_daily_limit: 7.9,
      }),
    ).toEqual({
      subnexus_invite_rewards_enabled: true,
      subnexus_invite_reward_inviter_amount: 1.25,
      subnexus_invite_reward_invitee_amount: 0.5,
      subnexus_invite_reward_ip_limit_enabled: true,
      subnexus_invite_reward_ip_daily_limit: 7,
    });
  });

  it("rejects invalid amounts and limits instead of enabling unsafe values", () => {
    expect(
      normalizeSubNexusInviteRewardSettings({
        subnexus_invite_rewards_enabled: "true" as unknown as boolean,
        subnexus_invite_reward_inviter_amount: -1,
        subnexus_invite_reward_invitee_amount: Number.NaN,
        subnexus_invite_reward_ip_limit_enabled: 1 as unknown as boolean,
        subnexus_invite_reward_ip_daily_limit: 0,
      }),
    ).toEqual(SUBNEXUS_INVITE_REWARD_DEFAULTS);
  });
});
