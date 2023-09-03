class Player:
    def __init__(self, data: dict[str, any]):
        self.name = data["name"]
        self.projection = data["projection"]
        self.actual = data["actual"]
        self.diff = data["diff"]
        self.position = data["position"]
        self.status = data["status"]

    def on_bench(self) -> bool:
        return self.status == "BE"

    def __str__(self):
        return f"Player({self.name}, {self.position}, {self.status}, proj: {self.projection}, actual: {self.actual})"
