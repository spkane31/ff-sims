import json


# TODO seankane: do a json merge if the file exists so I'm not overwriting the file all the time
def write_to_file(data: dict[str, any], file_name: str = "history.json") -> None:
    if len(data) == 0:
        return
    with open(file_name, mode="w") as f:
        json.dump(data, f, indent=4)
    return
