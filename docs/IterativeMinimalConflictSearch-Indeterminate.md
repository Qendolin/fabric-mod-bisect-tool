# **Algorithm: Iterative Minimal Conflict Search with Indeterminate Results (IMCS-I)**

The Iterative Minimal Conflict Search (IMCS) algorithm described in
[IterativeMinimalConflictSearch.md](IterativeMinimalConflictSearch.md) assumes a
binary test outcome: FAIL or GOOD. In practice a third outcome exists:
**INDETERMINATE**. This document describes the **IMCS-I** extension that
handles it.

## 1. Problem Statement

A secondary issue (for example, a crash on game launch) can prevent the user
from observing whether the primary issue is present. The test feedback
therefore needs a third answer: INDETERMINATE.

## 2. What Changed and Why

An INDETERMINATE result on `StableSet ∪ C₁` always means an **undeclared
dependency**: some component in C₁ silently relies on a component in C₂, and
the split separated them. (If the dependency were declared, the mod loader
would refuse to start with a clear error instead of an ambiguous crash — so
IMCS-I only needs to handle the undeclared case.)

IMCS already guarantees `test(StableSet ∪ CandidateSet) = FAIL` at every call.
So when `test(StableSet ∪ C₁) = INDETERMINATE`, we already know
`test((StableSet ∪ C₂) ∪ C₁) = FAIL` for free — no extra test needed. Folding
C₂ into StableSet before recursing into C₁ suppresses the secondary conflict
for the whole descent, so it doesn't reappear at every level.

This shortcut is only safe when we have one verified reading to lean on — i.e.
when exactly one half is INDETERMINATE. When *both* halves are INDETERMINATE,
there's no verified reading, and Section 4 covers how the procedure handles
that.

## 3. Updated Helper Procedure: `FindNextConflictElement`

```
1   function FindNextConflictElement(StableSet, CandidateSet):
2
3       if CandidateSet is empty:
4           return null
5
6       if size(CandidateSet) = 1:
7           let c be the single element in CandidateSet
8           if test(StableSet ∪ {c}) = FAIL:
9               return c
10          else:
11              // GOOD: not a conflict element.
12              // INDETERMINATE: c itself causes a secondary conflict;
13              // treat as a non-element for this search.
14              return null
15
16      Split CandidateSet into two halves C₁ and C₂.
17      result₁ ← test(StableSet ∪ C₁)
18
19      if result₁ = FAIL:
20          if size(C₁) = 1:
21              return the single element in C₁
22          return FindNextConflictElement(StableSet, C₁)
23
24      if result₁ = GOOD:
25          new_stable ← StableSet ∪ C₁
26          if size(C₂) = 1:
27              if test(new_stable ∪ C₂) = FAIL:
28                  return the element in C₂
29              else:
30                  return null
31          return FindNextConflictElement(new_stable, C₂)
32
33      // --- INDETERMINATE: C₁ has a split-induced secondary conflict. ---
34      if result₁ = INDETERMINATE:
35          result₂ ← test(StableSet ∪ C₂)
36
37          // Primary conflict element is in C₂. Proceed normally.
38          if result₂ = FAIL:
39              if size(C₂) = 1:
40                  return the element in C₂
41              return FindNextConflictElement(StableSet, C₂)
42
43          // C₂ is confirmed clean. Fold it into StableSet and recurse
44          // into C₁ — no extra test needed (see Section 2).
45          if result₂ = GOOD:
46              return FindNextConflictElement(StableSet ∪ C₂, C₁)
47
48          // Both halves are INDETERMINATE. Neither has a verified reading,
49          // so search both, each suppressing the other's secondary
50          // conflict. Return whichever finds an element first.
51          if result₂ = INDETERMINATE:
52              found ← FindNextConflictElement(StableSet ∪ C₂, C₁)
53              if found is not null:
54                  return found
55              return FindNextConflictElement(StableSet ∪ C₁, C₂)
```

The main procedure `FindConflictSet` and the `IMCS_Enumerator` are unchanged.

## 4. The Double-INDETERMINATE Case

Lines 51–55 handle the case where both halves are blocked by independent
secondary conflicts. This needs two unrelated undeclared dependencies to land
on opposite sides of the same split — e.g. `HudOverlayMod` (in C₁) needs
`CoreLibMod` (in C₂), while separately `ShaderCompatMod` (in C₂) needs
`RenderHookMod` (in C₁). Rare, but possible.

**Why it's costly:** neither half has a verified-clean reading, so neither
can be folded into StableSet for free — the algorithm has to actually search
both. That turns the recurrence for this branch from `T(n) = T(n/2) + O(1)`
into `T(n) = 2·T(n/2) + O(1)`, which is `O(n)` instead of `O(log n)` if it
recurs at every level.

**Why it's still correct:** no guessing happens — both halves are genuinely
searched, so the true conflict element is never missed.

### 4.1 Alternative: Halt Instead of Forking

If the `O(n)` worst case is unacceptable, lines 51–55 can be replaced with a
halt:

```
if result₂ = INDETERMINATE:
    report "Two independent secondary conflicts detected: " ∪ C₁ ∪ " and " ∪ C₂
    halt
```

This trades completeness for a hard complexity guarantee:

|                                 | Fork (current)                         | Halt (alternative)                                                                           |
| ------------------------------- | -------------------------------------- | -------------------------------------------------------------------------------------------- |
| Worst-case complexity           | O(n) if recurring at every level       | O(log n), always                                                                             |
| Extra automated tests per event | up to 2× the branch                    | none                                                                                         |
| Outcome                         | always finds the element if one exists | search stops there; user must resolve one of the two reported conflicts before it can resume |

So halt doesn't avoid the cost of double INDETERMINATE, it just relocates it:
instead of spending extra *tests*, you spend a stalled search that needs
manual intervention. Which is preferable depends on the tool's priorities —
fork if the search should always run to completion on its own, halt if a
predictable, bounded number of tests matters more than always finishing
unattended.

## 5. Cost Analysis

Each single-INDETERMINATE event costs one extra complement test, then the
recursion continues cleanly. For `q` single-INDETERMINATE events, total cost
is:

**O((p + q) log n)**

Double-INDETERMINATE events are the exception: with the fork (Section 3),
each one can roughly double the cost of its branch, and in the worst case
(recurring at every level) the whole search degrades to `O(n)`. With the halt
alternative (Section 4.1), double-INDETERMINATE events cost nothing extra but
can end the search early.