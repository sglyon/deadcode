import os
import json  # unused

USED_CONST = 1
UNUSED_CONST = 2  # unused


def used_function():
    return USED_CONST


def unused_helper(x):
    return x * 2


class UsedClass:
    def __init__(self):
        self.value = 10

    def used_method(self):
        return self.value

    def unused_method(self):
        return 99


class UnusedClass:
    pass


def main():
    obj = UsedClass()
    print(obj.used_method())
    print(used_function())
    print(os.getcwd())


if __name__ == "__main__":
    main()
