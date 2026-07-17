# Channel Balance Protection

This context defines how a channel's model availability changes when its upstream balance becomes too low for normal paid usage.

## Language

**Balance Protection**:
A per-channel, opt-in policy that limits model availability while the upstream balance is below a configured threshold. A protected channel remains enabled at zero balance so its free models can remain available.
_Avoid_: Low-balance disable, channel shutdown

**Free Model Allowlist**:
The exact model identifiers that remain available through a channel while Balance Protection is active. Models outside the allowlist are treated as chargeable and unavailable through that channel.
_Avoid_: Paid-model blacklist, excluded models

**Exposed Model**:
A model identifier accepted from callers and used for channel routing. When model mapping exists, it is the mapping source rather than the upstream model identifier.
_Avoid_: Upstream model, mapped target

**Discovered Model**:
An upstream model found by model discovery that is not yet an Exposed Model. Selecting a Discovered Model for the Free Model Allowlist also promotes it to an Exposed Model.
_Avoid_: Enabled model, routable model

**Invalid Free Model Allowlist**:
A Free Model Allowlist with no entries that are currently Exposed Models. Protected Routing permits no traffic while the allowlist is invalid.
_Avoid_: Empty protection, unrestricted fallback

**Free Model Classification**:
The administrator-confirmed designation that an Exposed Model is free on a specific upstream channel. Model discovery alone does not establish this designation.
_Avoid_: Discovered model, zero-priced local model

**Protection Threshold**:
The balance below which Balance Protection becomes active. Its default value is USD 2.
_Avoid_: Disable threshold, zero-balance threshold

**Recovery Threshold**:
The balance at or above which Balance Protection becomes inactive. Its default value is USD 5 and it must be higher than the Protection Threshold.
_Avoid_: Enable threshold, reset threshold

**Unknown Balance**:
The state in which a channel has failed ten consecutive balance checks. Unknown Balance activates Balance Protection; any successful balance check clears the consecutive-failure count.
_Avoid_: Zero balance, unavailable channel

**Pending Balance Verification**:
The protected state immediately after Balance Protection is enabled and before the first successful balance check. Paid models remain unavailable until recovery is confirmed.
_Avoid_: Unknown Balance, initial balance

**Balance Check Interval**:
The per-channel interval between scheduled balance checks. It defaults to one minute and may be configured from one to sixty minutes.
_Avoid_: Task interval, global refresh frequency

**Protected Routing**:
The temporary exclusion of a protected channel from routing for models outside its Free Model Allowlist. It does not change the channel's configured models or enabled status.
_Avoid_: Model removal, channel disablement

**Pinned Channel**:
A channel explicitly bound to a token or request and therefore not eligible for automatic fallback. Protected Routing rejects non-allowlisted models on a Pinned Channel while allowing its free models.
_Avoid_: Preferred channel, channel affinity

**Protection Transition**:
A change into or out of Protected Routing, including transitions caused by low balance, Unknown Balance, recovery, or an invalid allowlist. Administrator notifications are emitted once per transition rather than once per balance check.
_Avoid_: Balance update, polling notification

**Balance Exhaustion Error**:
An upstream response positively identified as a balance or quota exhaustion failure. It activates Balance Protection and fallback rather than disabling the entire channel.
_Avoid_: Authentication failure, general channel error
