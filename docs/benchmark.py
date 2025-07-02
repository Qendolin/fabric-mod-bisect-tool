import random
import statistics
from typing import Set, List, Optional
import matplotlib.pyplot as plt
import matplotlib.ticker as mticker
import numpy as np


# ==============================================================================
# The "Oracle" - Simulates the expensive test (e.g., launching Minecraft)
# ==============================================================================
class TestRunner:
    """
    A deterministic test oracle that knows the secret problematic set.
    It returns True (FAIL) if the test_set is a superset of the secret set.
    It also counts how many times it has been called.
    """
    def __init__(self, problematic_set: Set[int]):
        self._problematic_set = problematic_set
        self.test_count = 0

    def run(self, test_set: Set[int]) -> bool:
        """Runs the test. Returns True for FAIL (F), False for GOOD (G)."""
        self.test_count += 1
        return self._problematic_set.issubset(test_set)

# ==============================================================================
# Algorithm 1: Iterative Additive Search
# ==============================================================================

def _find_one_culprit_recursive(
    oracle: TestRunner,
    base_set: Set[int],
    search_pool: List[int]
) -> Optional[int]:
    """
    Recursively binary-searches the search_pool to find the *single* mod
    that, when added to base_set, causes a failure.
    """
    if not search_pool:
        return None
        
    if len(search_pool) == 1:
        # Final check if the single item is the culprit
        if oracle.run(base_set.union(search_pool)):
            return search_pool[0]
        else:
            return None

    mid = len(search_pool) // 2
    first_half = search_pool[:mid]
    second_half = search_pool[mid:]

    # Test the first half combined with the base set
    if oracle.run(base_set.union(first_half)):
        return _find_one_culprit_recursive(oracle, base_set, first_half)
    else:
        # The first half is "good", so add it to the base set for the next check
        return _find_one_culprit_recursive(oracle, base_set.union(first_half), second_half)


def find_conflicts_additive(oracle: TestRunner, all_mods: List[int]) -> Set[int]:
    """
    Finds the minimal failing set by iteratively finding one culprit at a time.
    """
    if not oracle.run(set(all_mods)):
        return set()

    confirmed_culprits: Set[int] = set()
    candidates = list(all_mods)

    while not oracle.run(confirmed_culprits):
        next_culprit = _find_one_culprit_recursive(
            oracle,
            base_set=confirmed_culprits,
            search_pool=candidates
        )
        
        if next_culprit is not None:
            confirmed_culprits.add(next_culprit)
            candidates.remove(next_culprit)
        else:
            raise RuntimeError("Additive search failed to find the next culprit.")

    return confirmed_culprits


# ==============================================================================
# Algorithm 2: The "Smart Additive" Algorithm (IterativeMinimalConflictSearch)
# ==============================================================================


def _find_next_conflict_element_optimized(oracle: TestRunner, background: Set[str], candidates: List[str]) -> Optional[str]:
    # Ensure candidates are sorted for deterministic splitting
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
            # Recursive call
            return _find_next_conflict_element_optimized(oracle, background, list(c1))
    
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
            # Recursive call
            return _find_next_conflict_element_optimized(oracle, new_background, list(c2))

def find_conflicts_smart_additive(oracle: TestRunner, all_mods: List[int]) -> Set[int]:
    """
    The Iterative Minimal Conflict Search (IMCS) algorithm.
    """
    conflict_set: Set[str] = set()
    candidates: List[str] = sorted(all_mods) # Ensure initial candidates are sorted

    while True:
        # Find the next single component that, in conjunction with the current conflict_set, 
        # contributes to the failure
        next_element = _find_next_conflict_element_optimized(oracle, conflict_set, candidates)
        
        if next_element is None:
            # If no additional conflict element can be found, the process is complete.
            break
        
        # Add the found element to the confirmed conflict_set and remove it from candidates.
        conflict_set.add(next_element)
        candidates.remove(next_element)
        
        # Optimization: Test if the current conflict_set is already a complete, minimal set.
        # If it causes failure, we can terminate early.
        if oracle.run(conflict_set):
            break
            
    return conflict_set

# ==============================================================================
# Algorithm 3: Classic `ddmin` (Subtractive)
# ==============================================================================

def find_conflicts_subtractive_ddmin(oracle: TestRunner, all_mods: List[int]) -> Set[int]:
    """
    Finds the minimal failing set using the classic ddmin algorithm.
    Starts with a failing set and tries to remove pieces.
    """
    test_set = set(all_mods)

    if not oracle.run(test_set):
        return set()

    granularity = 2
    while len(test_set) >= 2:
        mods_list = sorted(list(test_set))
        partitions = [mods_list[i::granularity] for i in range(granularity)]
        
        made_progress = False
        
        for p in partitions:
            if not p: continue # Skip empty partitions
            complement = test_set.difference(p)
            if oracle.run(complement):
                test_set = complement
                granularity = 2
                made_progress = True
                break

        if made_progress:
            continue

        if granularity < len(test_set):
            granularity = min(len(test_set), granularity * 2)
        else:
            break
            
    return test_set

# ==============================================================================
# Algorithm 4: QuickXplain
# ==============================================================================
def _quickxplain_recursive(oracle: TestRunner, background: Set[int], candidates: Set[int]) -> Set[int]:
    """The recursive core of the QuickXplain algorithm."""
    # Base Case 1: If C is empty or B already fails, no explanation from C is needed.
    if not candidates or oracle.run(background):
        return set()
    
    # Base Case 2: If C has a single element, it must be the explanation.
    if len(candidates) == 1:
        return candidates

    # Recursive Step: Divide and Conquer
    candidate_list = list(candidates)
    mid = len(candidate_list) // 2
    c1 = set(candidate_list[:mid])
    c2 = set(candidate_list[mid:])

    # Filter Pass: Find conflicts in c2, assuming all of c1 is present.
    cs2 = _quickxplain_recursive(oracle, background.union(c1), c2)
    
    # Refine Pass: Find conflicts in c1, assuming only the essential parts of c2 are present.
    cs1 = _quickxplain_recursive(oracle, background.union(cs2), c1)

    return cs1.union(cs2)

def find_conflicts_qxp(oracle: TestRunner, all_mods: List[int]) -> Set[int]:
    """Finds the minimal failing set using the QuickXplain algorithm."""
    # Initial check: if the full set doesn't fail, there's nothing to find.
    if not oracle.run(set(all_mods)):
        return set()
    
    return _quickxplain_recursive(oracle, set(), set(all_mods))

# ==============================================================================
# The "Adaptive" Algorithm - The Best of Both Worlds
# ==============================================================================
ADAPTIVE_THRESHOLD = 50 # The number of candidates below which we switch to the QXP strategy

def _adaptive_recursive(oracle: TestRunner, background: Set[int], candidates: Set[int]) -> Set[int]:
    """The recursive core of the Adaptive algorithm."""
    # Base Case 1: If C is empty or B already fails, no explanation from C is needed.
    if not candidates or oracle.run(background):
        return set()
    
    # --- The Adaptive Strategy Switch ---
    if len(candidates) <= ADAPTIVE_THRESHOLD:
        # STRATEGY 1: For small, dense sets, use the powerful QuickXplain logic.
        if len(candidates) == 1: return candidates # QXP Base Case
        candidate_list = list(candidates)
        mid = len(candidate_list) // 2
        c1, c2 = set(candidate_list[:mid]), set(candidate_list[mid:])
        cs2 = _adaptive_recursive(oracle, background.union(c1), c2)
        cs1 = _adaptive_recursive(oracle, background.union(cs2), c1)
        return cs1.union(cs2)
    else:
        # STRATEGY 2: For large, sparse sets, use the lean Smart Additive logic.
        # Find just one culprit from the current candidates.
        next_culprit = _find_one_culprit_recursive(oracle, background, list(candidates))
        
        if next_culprit is None:
            return set() # No single culprit found, means no culprits are in C given B
        
        # We found one. The final conflict set is this culprit plus whatever is found
        # by recursively searching the rest of the candidates.
        remaining_candidates = candidates - {next_culprit}
        return {next_culprit}.union(_adaptive_recursive(oracle, background.union({next_culprit}), remaining_candidates))


def find_conflicts_adaptive(oracle: TestRunner, all_mods: List[int]) -> Set[int]:
    """Finds the minimal failing set using a hybrid of Smart Additive and QuickXplain logic."""
    if not oracle.run(set(all_mods)):
        return set()
    return _adaptive_recursive(oracle, set(), set(all_mods))

# ==============================================================================
# Benchmarking and Plotting Harness
# ==============================================================================
def run_benchmark_and_plot():
    if not plt or not np:
        print("Matplotlib and/or numpy not found. Cannot generate plots.")
        return

    N_VALUES = sorted(list(set([5, 10, 25, 50, 75, 100, 150, 200, 250, 300, 400, 500])))
    P_MAX = 5
    TRIALS_PER_CONFIG = 500

    algorithms = {
        "Original Additive": find_conflicts_additive,
        "Smart Additive": find_conflicts_smart_additive,
        "ddmin Subtractive": find_conflicts_subtractive_ddmin,
        "QuickXplain": find_conflicts_qxp,
        "Adaptive Hybrid": find_conflicts_adaptive
    }
    
    results = {p: {
        'n': N_VALUES,
        **{name: {'all_trials': [[] for _ in N_VALUES]} for name in algorithms}
    } for p in range(1, P_MAX + 1)}

    print("--- Starting Benchmark ---")
    total_configs = len(range(1, P_MAX + 1)) * len(N_VALUES)
    current_config = 0
    
    for p in range(1, P_MAX + 1):
        for i, n in enumerate(N_VALUES):
            current_config += 1
            print(f"  Running config {current_config}/{total_configs} (p={p}, n={n})...", end="\r", flush=True)
            for name, func in algorithms.items():
                trial_counts_for_alg = []
                for _ in range(TRIALS_PER_CONFIG):
                    all_mods = list(range(n))
                    if len(all_mods) < p: continue
                    problematic_set = set(random.sample(all_mods, k=p))
                    oracle = TestRunner(problematic_set)
                    found = func(oracle, all_mods)
                    trial_counts_for_alg.append(oracle.test_count)
                    if found != problematic_set:
                        raise ValueError(f"VALIDATION FAILED: {name} for n={n}, p={p}, Expected: {problematic_set}, Got: {found}")
                results[p][name]['all_trials'][i] = trial_counts_for_alg

    print("\n\n--- Benchmark Complete ---")

    print("\n--- Test Counts Summary (min / median / max / avg) ---")
    def get_stats_str(counts):
        if not counts: return "N/A"
        s_min, s_med, s_max, s_avg = min(counts), statistics.median(counts), max(counts), statistics.mean(counts)
        return f"{s_min:<4} / {s_med:<5.0f} / {s_max:<5} / {s_avg:<6.1f}"

    col_width = 33
    header = f"{'p':<2} | {'n':<4} || " + " | ".join([f"{name:<{col_width}}" for name in algorithms])
    print(header)
    print("-" * len(header))
    for p in range(1, P_MAX + 1):
        for i, n in enumerate(N_VALUES):
            stats_line = f"{p:<2} | {n:<4} || "
            stats_parts = [get_stats_str(results[p][name]['all_trials'][i]) for name in algorithms]
            stats_line += " | ".join(f"{s:<{col_width}}" for s in stats_parts)
            print(stats_line)
        if p < P_MAX: print("-" * len(header))

    # --- Plotting ---
    for p in range(1, P_MAX + 1):
        fig, ax = plt.subplots(figsize=(14, 8))
        colors = {"Original Additive": "orange", "ddmin Subtractive": "red", "Smart Additive": "green", "Smart Additive Opt": "lime", "QuickXplain": "blue", "Adaptive Hybrid": "purple"}
        num_algs = len(algorithms)
        group_spread_factor = 0.3
        min_y_val, max_y_val = float('inf'), 0
        
        for i, name in enumerate(algorithms):
            all_trials_for_alg = results[p][name]['all_trials']
            for trial_set in all_trials_for_alg:
                if trial_set: min_y_val, max_y_val = min(min_y_val, min(trial_set)), max(max_y_val, max(trial_set))

            offset_multiplier = 1 + (i - (num_algs - 1) / 2) * (group_spread_factor / num_algs)
            positions = [n * offset_multiplier for n in N_VALUES]
            box_width_factor = group_spread_factor / num_algs * 0.7
            widths = [n * box_width_factor for n in N_VALUES]

            ax.boxplot(all_trials_for_alg, positions=positions, widths=widths, patch_artist=True,
                       boxprops=dict(facecolor=colors[name], alpha=0.6), medianprops=dict(color='black', linewidth=2),
                       whiskerprops=dict(color='black', linewidth=1.5), capprops=dict(color='black', linewidth=1.5),
                       showfliers=False, zorder=5)
            
            avg_data = [statistics.mean(trials) if trials else 0 for trials in all_trials_for_alg]
            ax.plot(positions, avg_data, color=colors[name], lw=2.5, marker='x', markeredgecolor='black',
                    markersize=7, label=f'{name} (Avg)', zorder=10)
            
            if np:
                valid_indices = [idx for idx, data in enumerate(avg_data) if data > 0]
                if len(valid_indices) > 1:
                    n_fit, avg_fit = np.array(N_VALUES)[valid_indices], np.array(avg_data)[valid_indices]
                    coeffs = np.polyfit(np.log(n_fit), avg_fit, 1)
                    n_smooth = np.linspace(n_fit[0], n_fit[-1], 200)
                    trend_line = coeffs[0] * np.log(n_smooth) + coeffs[1]
                    ax.plot(n_smooth, trend_line, '--', color=colors[name], lw=1.2, alpha=1.0, zorder=8, label=f'{name} (Trend)')

        ax.set_title(f'Algorithm Performance for p = {p} Problematic Mods', fontsize=16)
        ax.set_xlabel('n (Total Number of Mods)', fontsize=12)
        ax.set_ylabel('Number of Tests (Log-2 Scale)', fontsize=12)
        ax.set_xscale('log')
        ax.set_yscale('log', base=2)
        ax.legend()
        ax.grid(True, which='both', linestyle='--', linewidth=0.5)
        
        ax.set_xlim(left=N_VALUES[0] * 0.7, right=N_VALUES[-1] * 1.4)
        ax.set_ylim(bottom=min_y_val * 0.9, top=max_y_val * 1.2)

        ax.xaxis.set_major_formatter(mticker.ScalarFormatter())
        ax.yaxis.set_major_formatter(mticker.ScalarFormatter())
        
        ax.set_xticks(N_VALUES)
        ax.set_xticklabels(N_VALUES, rotation=45, ha="right")
        ax.tick_params(axis='x', which='minor', bottom=False)
        fig.tight_layout()
    
    print("\nDisplaying plots...")
    plt.show()
                    
if __name__ == "__main__":
    run_benchmark_and_plot()