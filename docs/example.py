from typing import Set, List, Optional

# --- Test Oracle ---
class TestRunner:
    def __init__(self, problematic_set: Set[str]):
        self._problematic_set = problematic_set
        self.test_count = 0
        print(f"--- Test Oracle Initialized ---")
        print(f"Problematic Set: {sorted(list(problematic_set))}")
        print(f"-------------------------------")

    def run(self, test_set: Set[str]) -> bool:
        self.test_count += 1
        result = self._problematic_set.issubset(test_set)
        print(f"  TEST #{self.test_count}: {sorted(list(test_set))} -> {'FAIL' if result else 'GOOD'}")
        return result

# --- OPTIMIZED FindNextConflictElement (Helper for IMCS) ---
def FindNextConflictElement(oracle: TestRunner, background: Set[str], candidates: List[str]) -> Optional[str]:
    candidates_list = sorted(list(candidates)) 

    # Base Case 1: No more candidates to test.
    if not candidates_list:
        return None
    
    # Base Case 2: Only one candidate left. Handles initial call if C_all has size 1.
    if len(candidates_list) == 1:
        c = candidates_list[0]
        if oracle.run(background.union({c})):
            return c
        else:
            return None

    # Recursive Step: Divide and conquer.
    mid = len(candidates_list) // 2
    c1 = set(candidates_list[:mid])
    c2 = set(candidates_list[mid:])

    # Test the first half.
    if oracle.run(background.union(c1)):
        # OPTIMIZATION: The conflict is in C1. If C1 is a single element, we are done.
        if len(c1) == 1:
            return list(c1)[0]
        else:
            return FindNextConflictElement(oracle, background, list(c1))
    
    # Otherwise, the first half is "safe." Search the second half.
    else:
        new_background = background.union(c1)
        # OPTIMIZATION: The conflict might be in C2. If C2 is a single element, test it directly.
        if len(c2) == 1:
            d = list(c2)[0]
            if oracle.run(new_background.union({d})):
                return d
            else:
                return None
        else:
            return FindNextConflictElement(oracle, new_background, list(c2))


# --- FindConflictSet (Main IMCS Algorithm - Unchanged) ---
def FindConflictSet(oracle: TestRunner, C_all: List[str]) -> Set[str]:
    print(f"\n--- Running FindConflictSet ---")
    conflict_set: Set[str] = set()
    candidates: List[str] = sorted(C_all)

    iteration = 0
    while True:
        iteration += 1
        print(f"\nITERATION {iteration}:")
        print(f"  Current ConflictSet: {sorted(list(conflict_set))}")
        print(f"  Remaining Candidates: {sorted(candidates)}")

        next_element = FindNextConflictElement(oracle, conflict_set, candidates)
        
        if next_element is None:
            print("  No more conflict elements found. Terminating search.")
            break
        
        print(f"  Found next_element: {next_element}")
        
        conflict_set.add(next_element)
        candidates.remove(next_element)
        
        print(f"  ConflictSet after adding {next_element}: {sorted(list(conflict_set))}")
        
        print("  Checking if current ConflictSet causes failure...")
        if oracle.run(conflict_set):
            print("  Current ConflictSet causes FAIL. Minimal set found.")
            break
        else:
            print("  Current ConflictSet causes GOOD. Continuing search.")

    print(f"\n--- FindConflictSet Complete ---")
    return conflict_set

# --- Example Execution ---
if __name__ == "__main__":
    all_components = [chr(ord('a') + i) for i in range(16)] # a, b, ..., p
    problematic_components = {'c', 'k', 'p'}

    oracle = TestRunner(problematic_components)
    found_conflict_set = FindConflictSet(oracle, all_components)

    print(f"\nAlgorithm Result: {sorted(list(found_conflict_set))}")
    print(f"Expected Result:  {sorted(list(problematic_components))}")
    if found_conflict_set == problematic_components:
        print(f"Result matches problematic set. Algorithm succeeded in {oracle.test_count} tests!")
    else:
        print("Result MISMATCH. Algorithm error.")