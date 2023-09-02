import math


# TODO seankane: there's a way to put these in a separate file and export them here but I don't remember, reference azure-sdk-for-python
def std_dev(arr: list[int | float]) -> int | float:
    if len(arr) == 0:
        return 0
    avg = sum(arr) / len(arr)
    sum_squares = sum([(x - avg) * (x - avg) for x in arr])
    return math.pow(sum_squares / len(arr), 0.5)


def mean(arr: list[int | float]) -> int | float:
    return sum(arr) / len(arr)
