import math

import numpy as np


def std_dev(arr: list[int | float]) -> int | float:
    if len(arr) == 0:
        return 0
    avg = sum(arr) / len(arr)
    sum_squares = sum([(x - avg) * (x - avg) for x in arr])
    return math.pow(sum_squares / len(arr), 0.5)


def mean(arr: list[int | float]) -> int | float:
    return sum(arr) / len(arr)


def sample_normal_distribution(mean: float, std_dev: float) -> float:
    return np.random.default_rng().normal(mean, std_dev, 1)[0]
