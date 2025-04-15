import marqo
from logger import logger

class Memory:
    def __init__(self):
        logger.info("Initializing Memory...")
        self.mq = marqo.Client(url="http://localhost:8882")
        if not self.check_for_index():
            self.create_memory_index()

    def check_for_index(self):
        logger.info("Checking for memory index...")
        try:
            self.mq.get_index("memories")
            logger.info("Memory index found...")
            return True
        except Exception as e:
            logger.info("Memory index not found...")
            return False
    
    def create_memory_index(self):
        logger.info("Creating memory index...")
        self.mq.create_index("memories", model="hf/e5-base-v2")

    def add_memories(self, memories):
        logger.info("Adding memories...")
        self.mq.index("memories").add_documents(memories, tensor_fields=["memory"])

    def retrieve_relevant_memories(self, query):
        logger.info("Retrieving relevant memories...")
        results = self.mq.index("memories").search(q=query)
        return [hit["memory"] for hit in results["hits"]]

if __name__ == "__main__":
    memory = Memory()
    memory.add_memories([
        {
            "memory": "I like to cook"
        },
    ])
    print(memory.retrieve_relevant_memories("I like to cook"))
