FROM python:3.9-slim
RUN apt-get update && apt-get install -y rtl-sdr multimon-ng && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY requirements.txt .
RUN pip install -r requirements.txt
COPY *.py ./
CMD ["python", "client.py"]
