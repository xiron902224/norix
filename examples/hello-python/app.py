import os
from flask import Flask

app = Flask(__name__)


@app.get("/")
def hello():
    return "hello from norix railpack python\n"


if __name__ == "__main__":
    port = int(os.environ.get("PORT", "8080"))
    app.run(host="0.0.0.0", port=port)
