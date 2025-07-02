# **Example Trace: Iterative Minimal Conflict Search (IMCS)**

Consider a total set of components `C_all = {a,b,c,d,e,f,g,h,i,j,k,l,m,n,o,p}` (16 components).
The actual minimal conflict set (unknown to the algorithm) is `{c, k, p}`.

**Initial State:**
`ConflictSet = {}`
`Candidates = {a,b,c,d,e,f,g,h,i,j,k,l,m,n,o,p}`

---

#### **Iteration 1: Finding the first conflict element**

(This step internally executes `FindNextConflictElement(SafeSet=ConflictSet, Candidates=Candidates)`.)

1. **Call `FindNextConflictElement(SafeSet={}, Candidates={a,b,c,d,e,f,g,h,i,j,k,l,m,n,o,p})`**

   * `Candidates` split: `C₁={a,b,c,d,e,f,g,h}`, `C₂={i,j,k,l,m,n,o,p}`.
   * Test `SafeSet ∪ C₁ = {} ∪ {a,b,c,d,e,f,g,h} = {a,b,c,d,e,f,g,h}`.
   * Result: **GOOD**. (Does not contain `{c,k,p}`).
   * Proceed to search `C₂`. Recursive call: `FindNextConflictElement(SafeSet={a,b,c,d,e,f,g,h}, Candidates={i,j,k,l,m,n,o,p})`.
2. **Call `FindNextConflictElement(SafeSet={a,b,c,d,e,f,g,h}, Candidates={i,j,k,l,m,n,o,p})`**

   * `Candidates` split: `C₁={i,j,k,l}`, `C₂={m,n,o,p}`.
   * Test `SafeSet ∪ C₁ = {a,b,c,d,e,f,g,h} ∪ {i,j,k,l} = {a,b,c,d,e,f,g,h,i,j,k,l}`.
   * Result: **FAIL**. (Contains `k`).
   * Proceed to search `C₁`. Recursive call: `FindNextConflictElement(SafeSet={a,b,c,d,e,f,g,h}, Candidates={i,j,k,l})`.
3. **Call `FindNextConflictElement(SafeSet={a,b,c,d,e,f,g,h}, Candidates={i,j,k,l})`**

   * `Candidates` split: `C₁={i,j}`, `C₂={k,l}`.
   * Test `SafeSet ∪ C₁ = {a,b,c,d,e,f,g,h} ∪ {i,j} = {a,b,c,d,e,f,g,h,i,j}`.
   * Result: **GOOD**. (Missing `k`).
   * Proceed to search `C₂`. Recursive call: `FindNextConflictElement(SafeSet={a,b,c,d,e,f,g,h,i,j}, Candidates={k,l})`.
4. **Call `FindNextConflictElement(SafeSet={a,b,c,d,e,f,g,h,i,j}, Candidates={k,l})`**

   * `Candidates` split: `C₁={k}`, `C₂={l}`.
   * Test `SafeSet ∪ C₁ = {a,b,c,d,e,f,g,h,i,j} ∪ {k} = {a,b,c,d,e,f,g,h,i,j,k}`.
   * Result: **FAIL**. (Contains `k`).
   * Proceed to search `C₁`. Recursive call: `FindNextConflictElement(SafeSet={a,b,c,d,e,f,g,h,i,j}, Candidates={k})`.
5. **Call `FindNextConflictElement(SafeSet={a,b,c,d,e,f,g,h,i,j}, Candidates={k})`**

   * `Candidates` has size 1.
   * Test `SafeSet ∪ {k} = {a,b,c,d,e,f,g,h,i,j,k}`.
   * Result: **FAIL**.
   * Returns `k`.

**End of Iteration 1:**

- `next_element` in `FindConflictSet` is `k`.
- `ConflictSet` becomes `{k}`.
- `Candidates` becomes `{a,b,c,d,e,f,g,h,i,j,l,m,n,o,p}`.
- Test `ConflictSet = {k}`. Result: **GOOD**. (Missing `c`, `p`).
- The main loop continues.

---

#### **Iteration 2: Finding the second conflict element**

(This step internally executes `FindNextConflictElement(SafeSet={k}, Candidates={a,b,c,d,e,f,g,h,i,j,l,m,n,o,p})`.)

1. **Call `FindNextConflictElement(SafeSet={k}, Candidates={a,b,c,d,e,f,g,h,i,j,l,m,n,o,p})`**

   * `Candidates` split: `C₁={a,b,c,d,e,f,g}` and `C₂={h,i,j,l,m,n,o,p}`.
   * Test `SafeSet ∪ C₁ = {k} ∪ {a,b,c,d,e,f,g} = {a,b,c,d,e,f,g,k}`.
   * Result: **FAIL**. (Contains `c`).
   * Proceed to search `C₁`. Recursive call: `FindNextConflictElement(SafeSet={k}, Candidates={a,b,c,d,e,f,g})`.
2. **Call `FindNextConflictElement(SafeSet={k}, Candidates={a,b,c,d,e,f,g})`**

   * `Candidates` split: `C₁={a,b,c}` and `C₂={d,e,f,g}`.
   * Test `SafeSet ∪ C₁ = {k} ∪ {a,b,c} = {a,b,c,k}`.
   * Result: **FAIL**. (Contains `c`).
   * Proceed to search `C₁`. Recursive call: `FindNextConflictElement(SafeSet={k}, Candidates={a,b,c})`.
3. **Call `FindNextConflictElement(SafeSet={k}, Candidates={a,b,c})`**

   * `Candidates` split: `C₁={a}` and `C₂={b,c}`.
   * Test `SafeSet ∪ C₁ = {k} ∪ {a} = {a,k}`.
   * Result: **GOOD**. (Missing `c`).
   * Proceed to search `C₂`. Recursive call: `FindNextConflictElement(SafeSet={a,k}, Candidates={b,c})`.
4. **Call `FindNextConflictElement(SafeSet={a,k}, Candidates={b,c})`**

   * `Candidates` split: `C₁={b}` and `C₂={c}`.
   * Test `SafeSet ∪ C₁ = {a,k} ∪ {b} = {a,b,k}`.
   * Result: **GOOD**. (Missing `c`).
   * Proceed to search `C₂`. Recursive call: `FindNextConflictElement(SafeSet={a,b,k}, Candidates={c})`.
5. **Call `FindNextConflictElement(SafeSet={a,b,k}, Candidates={c})`**

   * `Candidates` has size 1.
   * Test `SafeSet ∪ {c} = {a,b,k} ∪ {c} = {a,b,c,k}`.
   * Result: **FAIL**.
   * Returns `c`.

**End of Iteration 2:**

- `next_element` in `FindConflictSet` is `c`.
- `ConflictSet` becomes `{c, k}`.
- `Candidates` becomes `{a,b,d,e,f,g,h,i,j,l,m,n,o,p}`.
- Test `ConflictSet = {c, k}`. Result: **GOOD**. (Missing `p`).
- The main loop continues.

---

#### **Iteration 3: Finding the third conflict element**

(This step internally executes `FindNextConflictElement(SafeSet={c,k}, Candidates={a,b,d,e,f,g,h,i,j,l,m,n,o,p})`.)

1. **Call `FindNextConflictElement(SafeSet={c,k}, Candidates={a,b,d,e,f,g,h,i,j,l,m,n,o,p})`**

   * `Candidates` split: `C₁={a,b,d,e,f,g}` and `C₂={h,i,j,l,m,n,o,p}`.
   * Test `SafeSet ∪ C₁ = {c,k} ∪ {a,b,d,e,f,g} = {a,b,c,d,e,f,g,k}`.
   * Result: **GOOD**. (Missing `p`).
   * Proceed to search `C₂`. Recursive call: `FindNextConflictElement(SafeSet={a,b,c,d,e,f,g,k}, Candidates={h,i,j,l,m,n,o,p})`.
2. **Call `FindNextConflictElement(SafeSet={a,b,c,d,e,f,g,k}, Candidates={h,i,j,l,m,n,o,p})`**

   * `Candidates` split: `C₁={h,i,j,l}` and `C₂={m,n,o,p}`.
   * Test `SafeSet ∪ C₁ = {a,b,c,d,e,f,g,k} ∪ {h,i,j,l} = {a,b,c,d,e,f,g,h,i,j,k,l}`.
   * Result: **GOOD**. (Missing `p`).
   * Proceed to search `C₂`. Recursive call: `FindNextConflictElement(SafeSet={a,b,c,d,e,f,g,h,i,j,k,l}, Candidates={m,n,o,p})`.
3. **Call `FindNextConflictElement(SafeSet={a,b,c,d,e,f,g,h,i,j,k,l}, Candidates={m,n,o,p})`**

   * `Candidates` split: `C₁={m,n}` and `C₂={o,p}`.
   * Test `SafeSet ∪ C₁ = {a,b,c,d,e,f,g,h,i,j,k,l} ∪ {m,n} = {a,b,c,d,e,f,g,h,i,j,k,l,m,n}`.
   * Result: **GOOD**. (Missing `p`).
   * Proceed to search `C₂`. Recursive call: `FindNextConflictElement(SafeSet={a,b,c,d,e,f,g,h,i,j,k,l,m,n}, Candidates={o,p})`.
4. **Call `FindNextConflictElement(SafeSet={a,b,c,d,e,f,g,h,i,j,k,l,m,n}, Candidates={o,p})`**

   * `Candidates` split: `C₁={o}` and `C₂={p}`.
   * Test `SafeSet ∪ C₁ = {a,b,c,d,e,f,g,h,i,j,k,l,m,n} ∪ {o} = {a,b,c,d,e,f,g,h,i,j,k,l,m,n,o}`.
   * Result: **GOOD**. (Missing `p`).
   * Proceed to search `C₂`. Recursive call: `FindNextConflictElement(SafeSet={a,b,c,d,e,f,g,h,i,j,k,l,m,n,o}, Candidates={p})`.
5. **Call `FindNextConflictElement(SafeSet={a,b,c,d,e,f,g,h,i,j,k,l,m,n,o}, Candidates={p})`**

   * `Candidates` has size 1.
   * Test `SafeSet ∪ {p} = {a,b,c,d,e,f,g,h,i,j,k,l,m,n,o,p}`.
   * Result: **FAIL**.
   * Returns `p`.

**End of Iteration 3:**

- `next_element` in `FindConflictSet` is `p`.
- `ConflictSet` becomes `{c, k, p}`.
- `Candidates` becomes `{a,b,d,e,f,g,h,i,j,l,m,n,o}`.
- Test `ConflictSet = {c, k, p}`. Result: **FAIL**. (This is the full problematic set).
- The main loop in `FindConflictSet` terminates.

**Final Result:** The algorithm identifies the minimal conflict set as `{c, k, p}`.
