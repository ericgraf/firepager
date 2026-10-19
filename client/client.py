import subprocess
import pika
import os
import json
import time
import logging
from datetime import datetime, timezone

# Setup logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')
logger = logging.getLogger(__name__)

def connect_rabbitmq():
    rabbit_url = os.environ.get('RABBITMQ_URL', 'amqp://guest:guest@firepager-rmq:5672/')
    parameters = pika.URLParameters(rabbit_url)
    while True:
        try:
            logger.info(f"Connecting to RabbitMQ at {rabbit_url}...")
            connection = pika.BlockingConnection(parameters)
            channel = connection.channel()
            channel.queue_declare(queue='pager_messages', durable=True)
            logger.info("Successfully connected to RabbitMQ")
            return connection, channel
        except Exception as e:
            logger.error(f"Waiting for RabbitMQ... {e}")
            time.sleep(5)

def main():
    client_id = os.environ.get('CLIENT_ID', os.uname()[1])
    logger.info(f"Starting FirePager SDR Client (ID: {client_id})...")
    
    mock_mode = os.environ.get('MOCK_SDR', 'false').lower() == 'true'
    frequency = os.environ.get('PAGER_FREQ', '155603323') # Default frequency
    
    while True:
        try:
            conn, channel = connect_rabbitmq()
            
            if mock_mode:
                logger.info("Running in MOCK mode")
                mock_samples = [
                    "POCSAG512: Address:  930032  Function: 2  Alpha:   ID:gdsJ3D<LF>SMOKE VISIBLE INSIDE-FIRE <LF>15:55:48<LF>6525 BANK ST  Dv: 9  Sec: 91D <LF>Gd: 91D<LF>CENOTE RD & STONE SCHOOL RD<LF>PT91 PT93 P92 LA91 TA92 TA94<LF>TA83 SC90 #36719<LF>Sent by: FDP2<EOT><NUL>",
                    "POCSAG512: Address:  910000  Function: 3  Alpha:   ID:IMzuFU<LF>2ND PAGE STRUCTURE FIRE AT 6525<LF>BANK ST<LF>Sent by: FDP1<EOT><NUL>",
                    "POCSAG512: Address:    1296  Function: 2  Alpha:   ID:0VFZuQ<LF>MVC-ROLLOVER  16:35:53<LF>3168 DALMAC RD  Dv: 9  Sec: 92B<LF> Gd: 92B<LF>FOREST RD & SPRINGHILL RD<LF>P92 RE93 SO23 TA92 SC90 #37196<LF>Sent by: FDP1<EOT>"
                ]
                while True:
                    import random
                    raw_line = random.choice(mock_samples)
                    msg = {
                        "client_id": client_id,
                        "timestamp": datetime.now(timezone.utc).isoformat(),
                        "raw_text": raw_line
                    }
                    try:
                        channel.basic_publish(
                            exchange='',
                            routing_key='pager_messages',
                            body=json.dumps(msg),
                            properties=pika.BasicProperties(delivery_mode=2)
                        )
                        logger.info(f"Sent raw mock msg from {client_id}")
                    except Exception as e:
                        logger.error(f"Failed to send mock msg: {e}")
                        break
                    time.sleep(10)
            else:
                # Real mode
                cmd = f"rtl_fm -f {frequency} -s 22050 -g 49- | multimon-ng -a POCSAG512 -a POCSAG1200 -a POCSAG2400 -f alpha -t raw /dev/stdin"
                logger.info(f"Starting SDR process with command: {cmd}")
                
                process = subprocess.Popen(cmd, shell=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
                
                for line in iter(process.stdout.readline, ''):
                    line = line.strip()
                    if not line:
                        continue
                    
                    msg = {
                        "client_id": client_id,
                        "timestamp": datetime.now(timezone.utc).isoformat(),
                        "raw_text": line
                    }
                    
                    try:
                        channel.basic_publish(
                            exchange='',
                            routing_key='pager_messages',
                            body=json.dumps(msg),
                            properties=pika.BasicProperties(delivery_mode=2)
                        )
                        logger.info(f"Sent raw line from {client_id}")
                    except Exception as e:
                        logger.error(f"Failed to send: {e}")
                        break

        except Exception as e:
            logger.error(f"Main loop error: {e}")
            time.sleep(10)

if __name__ == '__main__':
    main()
