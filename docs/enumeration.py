import random
import statistics
from typing import Set, List, Optional, Callable, Tuple
import matplotlib.pyplot as plt
import matplotlib.ticker as mticker
import itertools

# ==============================================================================
# 1. Core Components (Oracle and KnowledgeBase)
# ==============================================================================

class TestRunner:
    """A test oracle that knows multiple, independent conflict sets."""
    def __init__(self, problematic_sets: List[Set[str]]):
        self.problematic_sets = problematic_sets
        self.real_test_count = 0

    def run_real_test(self, test_set: Set[str]) -> bool:
        """Performs the expensive system test. Returns True (FAIL) or False (GOOD)."""
        self.real_test_count += 1
        for conflict in self.problematic_sets:
            if conflict.issubset(test_set):
                return True # FAIL
        return False # GOOD

class KnowledgeBase:
    """A memoization layer that wraps the TestRunner to cache results."""
    def __init__(self, oracle: TestRunner):
        self.oracle = oracle
        self.cache: dict[frozenset[str], bool] = {}

    def test(self, test_set: Set[str]) -> bool:
        """The cached test function. Checks cache first, otherwise calls the real oracle."""
        key = frozenset(test_set)
        if key in self.cache:
            return self.cache[key]
        
        result = self.oracle.run_real_test(test_set)
        self.cache[key] = result
        return result

# ==============================================================================
# 2. The Core IMCS Algorithm (Adapted to be modular)
# ==============================================================================

def _find_next_conflict_element_optimized(
    test_func: Callable[[Set[str]], bool],
    background: Set[str],
    candidates: List[str]
) -> Optional[str]:
    """The optimized recursive helper, now using a generic test function."""
    candidates_list = sorted(list(candidates)) 
    if not candidates_list: return None
    if len(candidates_list) == 1:
        c = candidates_list[0]
        return c if test_func(background.union({c})) else None

    mid = len(candidates_list) // 2
    c1, c2 = set(candidates_list[:mid]), set(candidates_list[mid:])

    if test_func(background.union(c1)):
        return list(c1)[0] if len(c1) == 1 else _find_next_conflict_element_optimized(test_func, background, list(c1))
    else:
        new_background = background.union(c1)
        if len(c2) == 1:
            d = list(c2)[0]
            return d if test_func(new_background.union({d})) else None
        else:
            return _find_next_conflict_element_optimized(test_func, new_background, list(c2))

def find_single_conflict_set(
    test_func: Callable[[Set[str]], bool],
    all_components: List[str]
) -> Set[str]:
    """The main IMCS procedure, adapted to find one conflict set using a generic test function."""
    conflict_set: Set[str] = set()
    candidates: List[str] = sorted(all_components)
    while True:
        next_element = _find_next_conflict_element_optimized(test_func, conflict_set, candidates)
        if next_element is None: break
        
        conflict_set.add(next_element)
        candidates.remove(next_element)
        
        if test_func(conflict_set): break
    return conflict_set

# ==============================================================================
# 3. The IMCS-Enumerator Meta-Procedure
# ==============================================================================

def find_all_conflict_sets_enumerator(all_components: List[str], oracle: TestRunner) -> List[Set[str]]:
    """The IMCS-Enum meta-procedure."""
    all_conflict_sets: List[Set[str]] = []
    candidates = set(all_components)
    knowledge_base = KnowledgeBase(oracle) # Initialize the shared cache ONCE.

    while True:
        # Run IMCS on the remaining candidates, passing the memoized test function.
        new_conflict_set = find_single_conflict_set(knowledge_base.test, list(candidates))
        
        if not new_conflict_set:
            break # No more conflicts found.
        
        all_conflict_sets.append(new_conflict_set)
        candidates -= new_conflict_set # Remove found elements from the next search.

        # Optimization: if the remaining candidates are clean, we can stop early.
        if not knowledge_base.test(candidates):
            break
            
    return all_conflict_sets

# ==============================================================================
# 4. Benchmarking and Plotting Harness
# ==============================================================================

def generate_disjoint_sets(pool: List[str], sizes: List[int]) -> List[Set[str]]:
    """Safely generates multiple disjoint sets of specified sizes from a pool."""
    available = list(pool)
    random.shuffle(available)
    result_sets = []
    
    total_needed = sum(sizes)
    if total_needed > len(available):
        raise ValueError(f"Not enough items in pool (len={len(available)}) to generate disjoint sets of total size {total_needed}.")
        
    start_index = 0
    for size in sizes:
        end_index = start_index + size
        result_sets.append(set(available[start_index:end_index]))
        start_index = end_index
        
    return result_sets

def generate_all_cartesian_products(max_p: int, num_sets: int) -> List[List[int]]:
    """Generates all combinations of conflict set sizes for N independent sets."""
    sizes_range = range(1, max_p + 1)
    return list(itertools.product(sizes_range, repeat=num_sets))

def run_enumerator_benchmark():
    if not plt:
        print("Matplotlib not found, cannot generate plots.")
        return

    N = 128
    TRIALS_PER_CONFIG = 1000
    all_components = [f"mod_{i:03}" for i in range(N)]

    # --- Define all test cases using Cartesian products and group them ---
    grouped_test_cases: dict[str, dict[str, List[int]]] = {
        "1 Independent Conflict": {},
        "2 Independent Conflicts": {},
        "3 Independent Conflicts": {},
    }

    # Single conflict sets
    for p_val in range(1, 9): # p=1 to 9
        key = f"p={p_val}"
        grouped_test_cases["1 Independent Conflict"][key] = [p_val]

    # Two independent conflicts (p1, p2 from 1 to 4)
    for p_combo in generate_all_cartesian_products(4, 2):
        key = "p={" + ",".join(map(str, p_combo)) + "}"
        grouped_test_cases["2 Independent Conflicts"][key] = list(p_combo)
    
    # Three independent conflicts (p1, p2, p3 from 1 to 3)
    for p_combo in generate_all_cartesian_products(3, 3):
        key = "p={" + ",".join(map(str, p_combo)) + "}"
        grouped_test_cases["3 Independent Conflicts"][key] = list(p_combo)
    
    # Sort keys for consistent output order and plotting
    # Sort by number of conflict sets, then by case name string
    sorted_test_case_keys_in_groups = {
        group_name: sorted(cases_in_group.keys()) 
        for group_name, cases_in_group in grouped_test_cases.items()
    }

    full_results_data: dict[str, dict[str, List[int]]] = {group_name: {} for group_name in grouped_test_cases}


    print(f"--- Starting IMCS-Enumerator Benchmark (n={N}, trials={TRIALS_PER_CONFIG}) ---")

    for group_name, cases_in_group_sorted in sorted_test_case_keys_in_groups.items():
        print(f"\nProcessing Group: {group_name}")
        for case_name in cases_in_group_sorted:
            conflict_sizes = grouped_test_cases[group_name][case_name]
            print(f"  Running case: {case_name} (total {sum(conflict_sizes)} mods)...")
            # Skip if total conflict mods exceed available N
            if sum(conflict_sizes) > N:
                print(f"    Skipping: Total problematic mods ({sum(conflict_sizes)}) exceeds N ({N}).")
                full_results_data[group_name][case_name] = [] # Store empty list for skipped cases
                continue

            test_counts = []
            for _ in range(TRIALS_PER_CONFIG):
                expected_sets = generate_disjoint_sets(all_components, conflict_sizes)
                oracle = TestRunner(expected_sets)
                
                found_sets = find_all_conflict_sets_enumerator(all_components, oracle)
                
                # Validation: Compare sets of frozensets for robust order-independent comparison
                if len(found_sets) != len(expected_sets) or set(map(frozenset, found_sets)) != set(map(frozenset, expected_sets)):
                    raise RuntimeError(
                        f"Validation failed for case {case_name}!\n"
                        f"Expected: {sorted(list(map(frozenset, expected_sets)))}\n"
                        f"Found:    {sorted(list(map(frozenset, found_sets)))}"
                    )

                test_counts.append(oracle.real_test_count)
            full_results_data[group_name][case_name] = test_counts

    print("\n--- Benchmark Complete ---")

    # --- Print Final Summary Table ---
    print("\n--- IMCS-Enumerator Test Counts Summary (min / median / max / avg) ---")
    # Adjust column width for "Conflict Case (Total=X)"
    max_case_label_len = max(len(f"{cn} (Total={sum(grouped_test_cases[gn][cn])})") 
                             for gn, group_cases in grouped_test_cases.items() 
                             for cn in group_cases)
    
    header_cols = [f"Conflict Case (Total=X):<{max(max_case_label_len, 25)}", "min", "median", "max", "avg"]
    header_line = " | ".join([f"{col:<{max(len(col), 5) if col != header_cols[0] else max_case_label_len}}" for col in header_cols])
    print(header_line)
    print("-" * len(header_line))

    for group_name in grouped_test_cases.keys():
        print(f"\n{group_name}:")
        for case_name in sorted_test_case_keys_in_groups[group_name]: # Ensure output order is consistent with generation
            counts = full_results_data[group_name][case_name]
            total_mods_in_case = sum(grouped_test_cases[group_name][case_name])
            case_label = f"{case_name} (Total={total_mods_in_case})"

            if not counts: # Handle skipped cases
                print(f"{case_label:<{max_case_label_len}} | {'N/A':<5} | {'N/A':<7} | {'N/A':<5} | {'N/A':<6}")
                continue

            s_min, s_med, s_max, s_avg = min(counts), statistics.median(counts), max(counts), statistics.mean(counts)
            print(f"{case_label:<{max_case_label_len}} | {s_min:<5} | {s_med:<7.0f} | {s_max:<5} | {s_avg:<6.1f}")
        
    # --- Plotting ---
    fig, ax = plt.subplots(figsize=(18, 9))
    
    global_min_y = float('inf')
    global_max_y = 0

    x_positions_by_group: dict[str, List[float]] = {}
    all_x_labels: List[str] = []
    
    current_x = 0
    group_spacing = 0.5 # Space between groups

    all_total_mod_groups: dict[int, List[Tuple[float, float]]] = {} # {total_mods: [(x_pos, avg_tests), ...]}

    for group_idx, (group_name, cases_in_group) in enumerate(grouped_test_cases.items()):
        sorted_case_names = sorted(cases_in_group.keys())
        
        for i, case_name in enumerate(sorted_case_names):
            case_x_pos = current_x + i * 1.0 # Position for this specific case
            
            # Store the x-position for this group's plot data
            if group_name not in x_positions_by_group:
                x_positions_by_group[group_name] = []
            x_positions_by_group[group_name].append(case_x_pos)
            
            all_x_labels.append(case_name)
            
            conflict_sizes = cases_in_group[case_name]
            total_mods = sum(conflict_sizes)
            if total_mods not in all_total_mod_groups:
                all_total_mod_groups[total_mods] = []
            
            if full_results_data[group_name][case_name]: # Only add if data exists
                avg_val = statistics.mean(full_results_data[group_name][case_name])
                all_total_mod_groups[total_mods].append((case_x_pos, avg_val))

        current_x += len(sorted_case_names) + group_spacing # Advance x for next group

    # Plotting box plots and average lines per group
    line_colors = ['red', 'blue', 'green'] # Colors for average lines for each group
    
    for group_idx, (group_name, cases_in_group) in enumerate(grouped_test_cases.items()):
        plot_data_for_group = []
        actual_x_positions = []
        for i, case_name in enumerate(sorted(cases_in_group.keys())): # Sorted for consistent order
            if full_results_data[group_name][case_name]: # Only plot if data exists
                plot_data_for_group.append(full_results_data[group_name][case_name])
                actual_x_positions.append(x_positions_by_group[group_name][i])

        if not plot_data_for_group: # Skip plotting empty groups
            continue

        # Update global min/max for Y-axis (for consistent scaling)
        for counts_list in plot_data_for_group:
             if counts_list:
                global_min_y = min(global_min_y, min(counts_list))
                global_max_y = max(global_max_y, max(counts_list))

        bp = ax.boxplot(plot_data_for_group, 
                        positions=actual_x_positions, 
                        widths=0.6, # Fixed width for boxes
                        patch_artist=True, 
                        showfliers=False, 
                        zorder=5) # Below average lines

        for patch in bp['boxes']:
            patch.set_facecolor(plt.get_cmap('Pastel1')(group_idx % plt.get_cmap('Pastel1').N)) # Subtle color per group
            patch.set_alpha(0.8)
        for median in bp['medians']:
            median.set_color('black')
            median.set_linewidth(2)
        for whisker in bp['whiskers']:
            whisker.set_color('black')
            whisker.set_linewidth(1.5)
        for cap in bp['caps']:
            cap.set_color('black')
            cap.set_linewidth(1.5)

        # Average line for this group
        averages = [statistics.mean(d) for d in plot_data_for_group]
        ax.plot(actual_x_positions, averages, 'o-', color=line_colors[group_idx % len(line_colors)], lw=2, markersize=8, label=f'{group_name} Avg')
    
    # --- Plotting Total Mod Count Lines ---
    line_color_total_mods = 'grey' # Subtle color
    # Custom legend for total mods lines to show only one entry
    total_mods_plotted = False
    for total_mods, points in all_total_mod_groups.items():
        if len(points) > 1: # Only plot lines if there's more than one point
            sorted_points = sorted(points, key=lambda p: p[0])
            x_coords = [p[0] for p in sorted_points]
            y_coords = [p[1] for p in sorted_points]
            if not total_mods_plotted: # Add label only once
                ax.plot(x_coords, y_coords, ':', color=line_color_total_mods, lw=1, alpha=0.6, zorder=3, label='Total Problematic Mods (Avg)')
                total_mods_plotted = True
            else:
                ax.plot(x_coords, y_coords, ':', color=line_color_total_mods, lw=1, alpha=0.6, zorder=3)


    ax.set_title(f'IMCS-Enumerator Performance (n={N})', fontsize=18)
    ax.set_xlabel('Conflict Set Configuration', fontsize=14)
    ax.set_ylabel('Total Number of Real Tests Performed', fontsize=14)
    
    # Set x-axis labels
    ax.set_xticks(range(len(all_x_labels)))
    ax.set_xticklabels(all_x_labels, rotation=45, ha="right", fontsize=10)
    
    ax.yaxis.set_major_locator(mticker.MaxNLocator(integer=True))
    ax.grid(True, which='major', linestyle='--', linewidth=0.5)
    
    # Create custom legend handles to avoid duplicate labels from total_mods lines
    handles, labels = ax.get_legend_handles_labels()
    
    # Deduplicate legend entries. The last label added for 'Total Problematic Mods (Avg)' will be the correct one.
    by_label = dict(zip(labels, handles)) 
    ax.legend(by_label.values(), by_label.keys(), fontsize=10)

    fig.tight_layout() # Adjust layout to prevent labels overlapping

    print("\nDisplaying plot...")
    plt.show()

if __name__ == "__main__":
    run_enumerator_benchmark()